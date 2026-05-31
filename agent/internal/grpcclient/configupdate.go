package grpcclient

import qfv1 "github.com/qf/qf/proto/qf/v1"

// ApplyConfigUpdate applies fields from a ConfigUpdate to the live components.
// All pointer params may be nil.
func ApplyConfigUpdate(
	cfg *qfv1.ConfigUpdate,
	hb *HeartbeatSender,
	eb *EventBatcher,
	cp *CounterPoller,
	fc *FlowEventCollector,
) {
	if cfg == nil {
		return
	}
	if hb != nil && cfg.HeartbeatIntervalMs > 0 {
		hb.SetIntervalMs(cfg.HeartbeatIntervalMs)
	}
	if eb != nil {
		if cfg.EventBatchSize > 0 {
			eb.SetBatchSize(cfg.EventBatchSize)
		}
		if cfg.EventBatchMaxAgeMs > 0 {
			eb.SetMaxAgeMs(cfg.EventBatchMaxAgeMs)
		}
	}
	if cp != nil && cfg.CounterReportIntervalMs > 0 {
		cp.SetIntervalMs(cfg.CounterReportIntervalMs)
	}
	if fc != nil {
		fc.SetEnabled(cfg.FlowEventsEnabled)
	}
}
