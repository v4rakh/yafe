//go:build embed

package frontend

import "embed"

//go:embed app/dist
var FrontendFS embed.FS
