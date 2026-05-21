package llm

import "github.com/johnny1110/evva/pkg/config"

// APIConfig is the per-provider credentials struct llm.Client factories
// receive. Aliased to pkg/config.APIConfig so the canonical definition
// can live in pkg/config (which pkg/llm and pkg/tools both depend on
// without forming a cycle).
type APIConfig = config.APIConfig
