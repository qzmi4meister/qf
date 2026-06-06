package loader

import "fmt"

// Config holds the runtime parameters stored in the qf_config BPF array map.
//
//	Slot 0: flags   bit0=ConntrackEnabled, bit1=FlowEventsEnabled
//	Slot 1: default ingress action (Action*)
//	Slot 2: default egress action  (Action*)
type Config struct {
	ConntrackEnabled  bool
	FlowEventsEnabled bool
	DefaultIngress    uint8 // Action* constant; 0 normalised to ActionAllow
	DefaultEgress     uint8 // Action* constant; 0 normalised to ActionAllow
}

// DefaultConfig is the zero-safe baseline written to the map on Load.
var DefaultConfig = Config{
	ConntrackEnabled:  false,
	FlowEventsEnabled: false,
	DefaultIngress:    ActionAllow,
	DefaultEgress:     ActionAllow,
}

// SetConfig writes cfg to the qf_config BPF map.
func SetConfig(objs *BpfObjects, cfg Config) error {
	flags := uint32(0)
	if cfg.ConntrackEnabled {
		flags |= 0x1
	}
	if cfg.FlowEventsEnabled {
		flags |= 0x2
	}
	if err := objs.QfConfig.Put(uint32(0), flags); err != nil {
		return fmt.Errorf("write config flags: %w", err)
	}

	ingress := uint32(cfg.DefaultIngress)
	if ingress == 0 {
		ingress = uint32(ActionAllow)
	}
	if err := objs.QfConfig.Put(uint32(1), ingress); err != nil {
		return fmt.Errorf("write default ingress: %w", err)
	}

	egress := uint32(cfg.DefaultEgress)
	if egress == 0 {
		egress = uint32(ActionAllow)
	}
	if err := objs.QfConfig.Put(uint32(2), egress); err != nil {
		return fmt.Errorf("write default egress: %w", err)
	}
	return nil
}

// GetConfig reads the current config from the qf_config BPF map.
func GetConfig(objs *BpfObjects) (Config, error) {
	var flags, ingress, egress uint32
	if err := objs.QfConfig.Lookup(uint32(0), &flags); err != nil {
		return Config{}, fmt.Errorf("read config flags: %w", err)
	}
	if err := objs.QfConfig.Lookup(uint32(1), &ingress); err != nil {
		return Config{}, fmt.Errorf("read default ingress: %w", err)
	}
	if err := objs.QfConfig.Lookup(uint32(2), &egress); err != nil {
		return Config{}, fmt.Errorf("read default egress: %w", err)
	}

	cfg := Config{
		ConntrackEnabled:  flags&0x1 != 0,
		FlowEventsEnabled: flags&0x2 != 0,
		DefaultIngress:    uint8(ingress),
		DefaultEgress:     uint8(egress),
	}
	if cfg.DefaultIngress == 0 {
		cfg.DefaultIngress = ActionAllow
	}
	if cfg.DefaultEgress == 0 {
		cfg.DefaultEgress = ActionAllow
	}
	return cfg, nil
}

// Loader convenience wrappers.

func (l *Loader) SetConfig(cfg Config) error {
	return SetConfig(&l.objs, cfg)
}

func (l *Loader) GetConfig() (Config, error) {
	return GetConfig(&l.objs)
}
