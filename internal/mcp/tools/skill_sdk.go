package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/diandian921/sofarpc-mcp/skills"
)

// SkillsExtensionName is the MCP Skills extension identifier a server declares in
// its capabilities to advertise skills/list and skills/get.
const SkillsExtensionName = "io.modelcontextprotocol/skills"

const (
	usingSofaRPCSkillName = "using-sofarpc"
	usingSofaRPCSkillURI  = "skill://" + usingSofaRPCSkillName + "/SKILL.md"
	skillMIMEType         = "text/markdown"
	skillResultType       = "complete"
	skillCacheScope       = "public"
)

// skillResource is one file of a skill: its URI plus the digest and size of the
// exact bytes resources/read returns for it.
type skillResource struct {
	URI    string `json:"uri"`
	Digest string `json:"digest"`
	Size   int    `json:"size"`
}

// skillEntry is the Skills extension description of one skill.
type skillEntry struct {
	URI         string            `json:"uri"`
	Frontmatter map[string]string `json:"frontmatter"`
	Resources   []skillResource   `json:"resources"`
}

type listSkillsParams struct {
	mcpsdk.ParamsBase
	Cursor string `json:"cursor,omitempty"`
}

type listSkillsResult struct {
	mcpsdk.ResultBase
	ResultType string       `json:"resultType"`
	Skills     []skillEntry `json:"skills"`
	TTLMs      int          `json:"ttlMs"`
	CacheScope string       `json:"cacheScope"`
}

type getSkillParams struct {
	mcpsdk.ParamsBase
	URI string `json:"uri"`
}

type getSkillResult struct {
	mcpsdk.ResultBase
	ResultType string     `json:"resultType"`
	Skill      skillEntry `json:"skill"`
	TTLMs      int        `json:"ttlMs"`
	CacheScope string     `json:"cacheScope"`
}

// AddSkills serves the embedded using-sofarpc skill through the MCP Skills
// extension: SKILL.md as a skill:// resource, plus the skills/list and skills/get
// methods that describe it. The server must also declare SkillsExtensionName in its
// capabilities. It panics on an invalid embedded skill, which is a build defect.
func AddSkills(srv *mcpsdk.Server) {
	entry, err := newSkillEntry(usingSofaRPCSkillName, usingSofaRPCSkillURI, skills.UsingSofaRPC)
	if err != nil {
		panic(err)
	}

	srv.AddResource(&mcpsdk.Resource{
		Name:        entry.Frontmatter["name"],
		URI:         entry.URI,
		Description: entry.Frontmatter["description"],
		MIMEType:    skillMIMEType,
	}, func(_ context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		return &mcpsdk.ReadResourceResult{
			Contents: []*mcpsdk.ResourceContents{
				{URI: entry.URI, MIMEType: skillMIMEType, Text: string(skills.UsingSofaRPC)},
			},
		}, nil
	})

	err = mcpsdk.AddReceivingCustomMethod(srv, "skills/list",
		func(_ context.Context, _ *mcpsdk.ServerSession, params *listSkillsParams) (*listSkillsResult, error) {
			if params != nil && params.Cursor != "" {
				return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "unknown cursor"}
			}
			return &listSkillsResult{
				ResultType: skillResultType,
				Skills:     []skillEntry{entry},
				CacheScope: skillCacheScope,
			}, nil
		})
	if err != nil {
		panic(err)
	}

	err = mcpsdk.AddReceivingCustomMethod(srv, "skills/get",
		func(_ context.Context, _ *mcpsdk.ServerSession, params *getSkillParams) (*getSkillResult, error) {
			if params == nil || params.URI != entry.URI {
				return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "unknown skill uri"}
			}
			return &getSkillResult{ResultType: skillResultType, Skill: entry, CacheScope: skillCacheScope}, nil
		})
	if err != nil {
		panic(err)
	}
}

// newSkillEntry describes a single-file skill whose SKILL.md is body. The skill
// name in the frontmatter must equal name, the skill path segment of uri.
func newSkillEntry(name, uri string, body []byte) (skillEntry, error) {
	frontmatter, err := parseSkillFrontmatter(body)
	if err != nil {
		return skillEntry{}, fmt.Errorf("skill %s: %w", name, err)
	}
	if frontmatter["name"] != name {
		return skillEntry{}, fmt.Errorf("skill %s: frontmatter name is %q", name, frontmatter["name"])
	}
	if frontmatter["description"] == "" {
		return skillEntry{}, fmt.Errorf("skill %s: frontmatter has no description", name)
	}
	sum := sha256.Sum256(body)
	return skillEntry{
		URI:         uri,
		Frontmatter: frontmatter,
		Resources: []skillResource{
			{URI: uri, Digest: "sha256:" + hex.EncodeToString(sum[:]), Size: len(body)},
		},
	}, nil
}

// parseSkillFrontmatter reads the leading YAML frontmatter of a SKILL.md. It accepts
// only flat `key: value` lines whose value is a plain scalar that YAML reads as that
// same string, and rejects everything else, so a field it cannot represent faithfully
// is an error rather than silently dropped or altered.
func parseSkillFrontmatter(body []byte) (map[string]string, error) {
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	if lines[0] != "---" {
		return nil, errors.New("missing frontmatter")
	}
	fields := map[string]string{}
	for _, line := range lines[1:] {
		if line == "---" {
			return fields, nil
		}
		key, value, ok := strings.Cut(line, ": ")
		value = strings.TrimSpace(value)
		if !ok || !frontmatterKey.MatchString(key) || !isPlainYAMLString(value) {
			return nil, fmt.Errorf("unsupported frontmatter line %q", line)
		}
		if _, dup := fields[key]; dup {
			return nil, fmt.Errorf("duplicate frontmatter key %q", key)
		}
		fields[key] = value
	}
	return nil, errors.New("unterminated frontmatter")
}

var frontmatterKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// isPlainYAMLString reports whether value is a single-line YAML plain scalar that
// resolves to a string equal to value: no quoting, indicators, comments or nested
// mapping syntax, and not a boolean, null or number.
func isPlainYAMLString(value string) bool {
	if value == "" || strings.ContainsAny(value[:1], "\"'|>[]{}&*!#%@`,?-~") {
		return false
	}
	if strings.Contains(value, " #") || strings.Contains(value, ": ") || strings.HasSuffix(value, ":") {
		return false
	}
	switch strings.ToLower(value) {
	case "true", "false", "yes", "no", "on", "off", "null":
		return false
	}
	_, err := strconv.ParseFloat(strings.ReplaceAll(value, "_", ""), 64)
	return err != nil
}
