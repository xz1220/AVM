package config

import "os"

func ReadGlobalConfig() (*GlobalConfig, error) {
	path := GlobalConfigPath()
	var cfg GlobalConfig
	if err := readYAML(path, &cfg); err != nil {
		return nil, err
	}
	cfg.ApplyDefaults()
	if err := validateGlobalConfig(&cfg, path); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func WriteGlobalConfig(cfg *GlobalConfig) error {
	path := GlobalConfigPath()
	if cfg == nil {
		return fieldError(path, "", "global config is nil")
	}
	cfg.ApplyDefaults()
	if err := validateGlobalConfig(cfg, path); err != nil {
		return err
	}
	return writeYAML(path, cfg)
}

func UpdateActive(ref ActiveRef) error {
	if err := validateActiveRef(ref, GlobalConfigPath()); err != nil {
		return err
	}

	cfg, err := ReadGlobalConfig()
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		cfg = &GlobalConfig{}
		cfg.ApplyDefaults()
	}
	cfg.Active = ref
	return WriteGlobalConfig(cfg)
}
