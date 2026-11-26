// Package skills embeds the agent skills shipped inside the binary.
package skills

import _ "embed"

// UsingSofaRPC is the SKILL.md of the using-sofarpc skill.
//
//go:embed using-sofarpc/SKILL.md
var UsingSofaRPC []byte
