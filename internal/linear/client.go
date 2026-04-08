package linear

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const apiEndpoint = "https://api.linear.app/graphql"

type Issue struct {
	Identifier string
	Title      string
	BranchName string // e.g. "eng-123-fix-auth-bug"; use as the git branch name
	State      WorkflowState
}

type WorkflowState struct {
	ID       string
	Name     string
	Type     string
	Color    string
	Position float64
}

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

var identifierRegex = regexp.MustCompile(`^([A-Z][A-Z0-9]+)-(\d+)$`)
var pickerIdentifierRegex = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]+)-(\d*)$`)

type parsedID struct {
	teamKey string
	number  int
}

type graphQLResponse[T any] struct {
	Data   T `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type issueNode struct {
	Identifier string         `json:"identifier"`
	Title      string         `json:"title"`
	BranchName string         `json:"branchName"`
	State      issueStateNode `json:"state"`
}

type issueConnection struct {
	Nodes []issueNode `json:"nodes"`
}

type issueStateNode struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Color    string  `json:"color"`
	Position float64 `json:"position"`
}

// FetchIssues retrieves Linear issues by their human-readable identifiers (e.g. ["ENG-123"]).
func (c *Client) FetchIssues(identifiers []string) (map[string]Issue, error) {
	var entries []parsedID
	seen := map[string]bool{}
	for _, id := range identifiers {
		if seen[id] {
			continue
		}
		seen[id] = true
		m := identifierRegex.FindStringSubmatch(id)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		entries = append(entries, parsedID{teamKey: m[1], number: n})
	}
	if len(entries) == 0 {
		return map[string]Issue{}, nil
	}

	var qb strings.Builder
	qb.WriteString("{")
	for i, e := range entries {
		qb.WriteString(fmt.Sprintf(
			` a%d: issues(first:1, filter:{team:{key:{eq:%q}},number:{eq:%d}}) { nodes { identifier title branchName state { id name type color position } } }`,
			i, e.teamKey, e.number,
		))
	}
	qb.WriteString(" }")

	body, err := json.Marshal(map[string]string{"query": qb.String()})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	result, err := c.doQueryMap(body)
	if err != nil {
		return nil, err
	}

	issues := make(map[string]Issue)
	for _, raw := range result {
		var connection issueConnection
		if err := json.Unmarshal(raw, &connection); err != nil {
			continue
		}
		for _, node := range connection.Nodes {
			issues[node.Identifier] = toIssue(node)
		}
	}
	return issues, nil
}

// FetchPickerIssues returns the first batch of issues for the picker.
// When query is non-empty, Linear filters results on the backend.
func (c *Client) FetchPickerIssues(query string) ([]Issue, error) {
	query = strings.TrimSpace(query)

	gql := `query PickerIssues($filter: IssueFilter) {
		issues(
			filter: $filter
			first: 50
			orderBy: updatedAt
		) {
			nodes {
				identifier
				title
				branchName
				state { id name type color position }
			}
		}
	}`

	variables := map[string]any{
		"filter": nil,
	}
	if query != "" {
		filters := []map[string]any{
			{"title": map[string]any{"containsIgnoreCase": query}},
		}
		if number, err := strconv.Atoi(query); err == nil {
			filters = append(filters, map[string]any{
				"number": map[string]any{"eq": number},
			})
		}
		if identifierFilter := pickerIdentifierFilter(query); identifierFilter != nil {
			filters = append(filters, identifierFilter)
		}
		variables["filter"] = map[string]any{"or": filters}
	}

	body, err := json.Marshal(map[string]any{
		"query":     gql,
		"variables": variables,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	var result graphQLResponse[struct {
		Issues issueConnection `json:"issues"`
	}]
	if err := c.doQuery(body, &result); err != nil {
		return nil, err
	}

	nodes := result.Data.Issues.Nodes
	issues := make([]Issue, 0, len(nodes))
	for _, node := range nodes {
		issues = append(issues, toIssue(node))
	}
	return issues, nil
}

func pickerIdentifierFilter(query string) map[string]any {
	matches := pickerIdentifierRegex.FindStringSubmatch(strings.TrimSpace(query))
	if matches == nil {
		return nil
	}

	teamKey := strings.ToUpper(matches[1])
	numberPart := matches[2]
	if numberPart == "" {
		return map[string]any{
			"team": map[string]any{
				"key": map[string]any{"eq": teamKey},
			},
		}
	}

	number, err := strconv.Atoi(numberPart)
	if err != nil {
		return nil
	}

	return map[string]any{
		"team": map[string]any{
			"key": map[string]any{"eq": teamKey},
		},
		"number": map[string]any{"eq": number},
	}
}

func (c *Client) doQueryMap(body []byte) (map[string]json.RawMessage, error) {
	var result graphQLResponse[map[string]json.RawMessage]
	if err := c.doQuery(body, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (c *Client) doQuery(body []byte, out any) error {
	req, err := http.NewRequest(http.MethodPost, apiEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	var envelope graphQLResponse[json.RawMessage]
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode errors: %w", err)
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("linear api: %s", envelope.Errors[0].Message)
	}
	return nil
}

func toIssue(node issueNode) Issue {
	return Issue{
		Identifier: node.Identifier,
		Title:      node.Title,
		BranchName: node.BranchName,
		State: WorkflowState{
			ID:       node.State.ID,
			Name:     node.State.Name,
			Type:     node.State.Type,
			Color:    node.State.Color,
			Position: node.State.Position,
		},
	}
}
