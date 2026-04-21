package event

// SovereignEvent is our clean, normalized output struct
type SovereignEvent struct {
	Timestamp   int64  `json:"ts_unix_ns"`
	PodName     string `json:"pod_name"`
	Namespace   string `json:"namespace"`
	Binary      string `json:"binary"`
	DestIP      string `json:"dest_ip"`
	DestPort    uint32 `json:"dest_port"`
	DestCountry string `json:"dest_country,omitempty"`
}
