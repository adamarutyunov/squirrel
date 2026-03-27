package linear

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	State      string
	BranchName string // e.g. "eng-123-fix-auth-bug"; use as the git branch name
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

type parsedID struct {
	teamKey string
	number  int
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
			` a%d: issues(first:1, filter:{team:{key:{eq:%q}},number:{eq:%d}}) { nodes { identifier title branchName state { name } } }`,
			i, e.teamKey, e.number,
		))
	}
	qb.WriteString(" }")

	body, err := json.Marshal(map[string]string{"query": qb.String()})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, apiEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data   map[string]json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("linear api: %s", result.Errors[0].Message)
	}

	type nodeResult struct {
		Identifier string `json:"identifier"`
		Title      string `json:"title"`
		BranchName string `json:"branchName"`
		State      struct {
			Name string `json:"name"`
		} `json:"state"`
	}
	type aliasResult struct {
		Nodes []nodeResult `json:"nodes"`
	}

	issues := make(map[string]Issue)
	for _, raw := range result.Data {
		var ar aliasResult
		if err := json.Unmarshal(raw, &ar); err != nil {
			continue
		}
		for _, node := range ar.Nodes {
			issues[node.Identifier] = Issue{
				Identifier: node.Identifier,
				Title:      node.Title,
				State:      node.State.Name,
				BranchName: node.BranchName,
			}
		}
	}
	return issues, nil
}

// FetchAssignedIssues returns open issues assigned to the authenticated user,
// ordered by most recently updated. Used to populate the context-creation picker.
func (c *Client) FetchAssignedIssues() ([]Issue, error) {
	const query = `{
		viewer {
			assignedIssues(
				filter: { state: { type: { nin: ["completed", "cancelled"] } } }
				first: 50
				orderBy: updatedAt
			) {
				nodes {
					identifier
					title
					branchName
					state { name }
				}
			}
		}
	}`

	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, apiEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Viewer struct {
				AssignedIssues struct {
					Nodes []struct {
						Identifier string `json:"identifier"`
						Title      string `json:"title"`
						BranchName string `json:"branchName"`
						State      struct {
							Name string `json:"name"`
						} `json:"state"`
					} `json:"nodes"`
				} `json:"assignedIssues"`
			} `json:"viewer"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("linear api: %s", result.Errors[0].Message)
	}

	nodes := result.Data.Viewer.AssignedIssues.Nodes
	issues := make([]Issue, 0, len(nodes))
	for _, node := range nodes {
		issues = append(issues, Issue{
			Identifier: node.Identifier,
			Title:      node.Title,
			State:      node.State.Name,
			BranchName: node.BranchName,
		})
	}
	return issues, nil
}
