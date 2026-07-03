// Command hivyapp is the app server: the hivycore platform core (auth,
// session, sheets client, static SPA serving) plus the agent-written API
// handlers registered in the api package.
package main

import (
	"os"

	"hivyapp/api"
	"hivyapp/hivycore"
)

func main() {
	app := hivycore.MustNew()
	api.Register(app)
	if err := app.Run(); err != nil {
		os.Exit(1)
	}
}
