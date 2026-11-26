package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
		func(_ context.Context, _ *mcpsdk.ServerSession, _ *listSkillsParams) (*listSkillsResult, error) {
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
// only flat `key: value` plain scalars and rejects any other line, so a field it
// cannot represent faithfully is an error rather than silently dropped.
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
		if !ok || key == "" || key != strings.TrimSpace(key) || value == "" || strings.ContainsAny(value[:1], `"'|>[{&*!`) {
			return nil, fmt.Errorf("unsupported frontmatter line %q", line)
		}
		fields[key] = value
	}
	return nil, errors.New("unterminated frontmatter")
}
