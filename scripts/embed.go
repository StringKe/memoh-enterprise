package scripts

import _ "embed"

// DisplayInstall is the fallback display install script bundled into the binary.
//
//go:embed display-install.sh
var DisplayInstall string
