package agentclient

//go:generate ../../../tools/oapi-codegen -config oapi-codegen.yaml ../../../api/agent-openapi.yaml
//go:generate sed -i "s|\"encoding/json\"|json \"github.com/goccy/go-json\"|" client-openapi.gen.go
