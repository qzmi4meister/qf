package loader

import "fmt"

// RuleCounters is the aggregated (summed across all CPUs) traffic counter
// for one rule entry.
type RuleCounters struct {
	Packets uint64
	Bytes   uint64
}

// ReadCounter returns the aggregated packet/byte count for rule at index idx.
func ReadCounter(objs *TcFilterObjects, idx uint32) (RuleCounters, error) {
	var perCPU []TcFilterRuleCounter
	if err := objs.QfRuleCounters.Lookup(idx, &perCPU); err != nil {
		return RuleCounters{}, fmt.Errorf("read counter[%d]: %w", idx, err)
	}
	var total RuleCounters
	for _, c := range perCPU {
		total.Packets += c.Packets
		total.Bytes += c.Bytes
	}
	return total, nil
}

// ReadCounters returns aggregated counters for all currently active rules.
// The slice is indexed by rule position (same order as the last PushRules call).
func ReadCounters(objs *TcFilterObjects) ([]RuleCounters, error) {
	var count uint32
	if err := objs.QfRuleCount.Lookup(uint32(0), &count); err != nil {
		return nil, fmt.Errorf("read rule_count: %w", err)
	}
	if count > MaxRules {
		count = MaxRules
	}
	out := make([]RuleCounters, count)
	for i := uint32(0); i < count; i++ {
		c, err := ReadCounter(objs, i)
		if err != nil {
			return nil, err
		}
		out[i] = c
	}
	return out, nil
}

// ReadSuppressedCount returns the total suppressed-event count summed across all
// CPUs since the last call to ResetSuppressedCount (or since agent start).
func ReadSuppressedCount(objs *TcFilterObjects) (uint64, error) {
	var perCPU []uint64
	if err := objs.QfSuppressedCount.Lookup(uint32(0), &perCPU); err != nil {
		return 0, fmt.Errorf("read suppressed_count: %w", err)
	}
	var total uint64
	for _, c := range perCPU {
		total += c
	}
	return total, nil
}

// ResetSuppressedCount zeroes the per-CPU suppressed-event counters.
// Call after ReadSuppressedCount to implement delta reporting.
func ResetSuppressedCount(objs *TcFilterObjects) error {
	var perCPU []uint64
	// Read to determine CPU count, then write zeroes.
	if err := objs.QfSuppressedCount.Lookup(uint32(0), &perCPU); err != nil {
		return fmt.Errorf("read suppressed_count for reset: %w", err)
	}
	for i := range perCPU {
		perCPU[i] = 0
	}
	if err := objs.QfSuppressedCount.Put(uint32(0), perCPU); err != nil {
		return fmt.Errorf("reset suppressed_count: %w", err)
	}
	return nil
}

// Loader convenience wrappers.

func (l *Loader) ReadCounter(idx uint32) (RuleCounters, error) {
	return ReadCounter(&l.objs, idx)
}

func (l *Loader) ReadCounters() ([]RuleCounters, error) {
	return ReadCounters(&l.objs)
}

func (l *Loader) ReadSuppressedCount() (uint64, error) {
	return ReadSuppressedCount(&l.objs)
}

func (l *Loader) ResetSuppressedCount() error {
	return ResetSuppressedCount(&l.objs)
}
