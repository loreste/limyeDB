package limyedb

type CollectionConfig struct {
	Name      string `json:"name"`
	Dimension int    `json:"dimension"`
	Metric    string `json:"metric,omitempty"`
}

type Point struct {
	ID           string                 `json:"id"`
	Vector       []float32              `json:"vector,omitempty"`
	NamedVectors map[string][]float32   `json:"named_vectors,omitempty"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
}

type Match struct {
	ID           string                 `json:"id"`
	Score        float32                `json:"score"`
	Vector       []float32              `json:"vector,omitempty"`
	NamedVectors map[string][]float32   `json:"named_vectors,omitempty"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
}

type ContextExample struct {
	ID           string               `json:"id,omitempty"`
	Vector       []float32            `json:"vector,omitempty"`
	NamedVectors map[string][]float32 `json:"named_vectors,omitempty"`
}

type ContextPair struct {
	Positive []ContextExample `json:"positive"`
	Negative []ContextExample `json:"negative,omitempty"`
}

type DiscoverParams struct {
	Target  []float32              `json:"target,omitempty"`
	Context *ContextPair           `json:"context,omitempty"`
	Limit   int                    `json:"limit,omitempty"`
	Ef      int                    `json:"ef,omitempty"`
	Filter  map[string]interface{} `json:"filter,omitempty"`
}
