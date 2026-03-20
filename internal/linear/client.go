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
// Linear's IssueFilter has no "identifier" field, so we build a single GraphQL request with
// one aliased sub-query per identifier, each filtering by team.key + number.
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
			` a%d: issues(first:1, filter:{team:{key:{eq:%q}},number:{eq:%d}}) { nodes { identifier title state { name } } }`,
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

	type aliasResult struct {
		Nodes []struct {
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
			State      struct {
				Name string `json:"name"`
			} `json:"state"`
		} `json:"nodes"`
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
			}
		}
	}
	return issues, nil
}
