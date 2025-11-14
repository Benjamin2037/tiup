package precheck

import (
	"embed"
	"fmt"

	"github.com/pingcap/tidb-upgrade-precheck/pkg/metadata"
)

//go:embed metadata/upgrade_changes.json
var upgradeMetadataFS embed.FS

func loadEmbeddedCatalog() (*metadata.Catalog, error) {
	data, err := upgradeMetadataFS.ReadFile("metadata/upgrade_changes.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded upgrade metadata: %w", err)
	}
	return metadata.LoadCatalogFromBytes(data)
}
