package packageio

const ManifestVersion = "1"

type Manifest struct {
	Version      string             `yaml:"version"`
	ExportedAt   string             `yaml:"exported_at"`
	Kind         string             `yaml:"kind"`
	Name         string             `yaml:"name"`
	Envs         []string           `yaml:"envs,omitempty"`
	Agents       []string           `yaml:"agents,omitempty"`
	Capabilities CapabilityManifest `yaml:"capabilities,omitempty"`
	IncludeFiles []string           `yaml:"include_files,omitempty"`
}

type CapabilityManifest struct {
	Skills   []string `yaml:"skills,omitempty"`
	MCPs     []string `yaml:"mcps,omitempty"`
	Commands []string `yaml:"commands,omitempty"`
	Hooks    []string `yaml:"hooks,omitempty"`
	Toolsets []string `yaml:"toolsets,omitempty"`
}

func (m CapabilityManifest) empty() bool {
	return len(m.Skills) == 0 &&
		len(m.MCPs) == 0 &&
		len(m.Commands) == 0 &&
		len(m.Hooks) == 0 &&
		len(m.Toolsets) == 0
}
