//go:build tools

package tools

// Keeps golang-migrate's CLI in the module graph for the Docker build.
import _ "github.com/golang-migrate/migrate/v4/cmd/migrate"
