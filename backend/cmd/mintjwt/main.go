// Command mintjwt is a test utility: it mints a session JWT for a given
// secret, user, and org. cli/e2e.sh uses it to drive the OAuth consent
// endpoint headlessly. It is useless without the server's SESSION_SECRET.
//
//	go run ./cmd/mintjwt <secret> <userID> <orgID> [role]
package main

import (
	"fmt"
	"os"

	"github.com/inboxes/backend/internal/middleware"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: mintjwt <secret> <userID> <orgID> [role]")
		os.Exit(2)
	}
	role := "admin"
	if len(os.Args) > 4 {
		role = os.Args[4]
	}
	token, _, err := middleware.GenerateToken(os.Args[1], os.Args[2], os.Args[3], role)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mint:", err)
		os.Exit(1)
	}
	fmt.Print(token)
}
