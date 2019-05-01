package main

import (
	"github.com/spiffe/spire/pkg/common/catalog"
	"github.com/spiffe/spire/proto/spire/server/datastore"

	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

const pluginName = "kubernetes"

func main() {
	catalog.PluginMain(
		catalog.MakePlugin(pluginName, datastore.PluginServer(New())),
	)
}
