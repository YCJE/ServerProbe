module github.com/server-probe/agent

go 1.26.4

require (
	github.com/gorilla/websocket v1.5.3
	github.com/prometheus-community/pro-bing v0.9.0
	github.com/server-probe/shared v0.0.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
)

replace github.com/server-probe/shared => ../shared
