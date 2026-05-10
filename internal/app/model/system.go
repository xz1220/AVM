package model

// InitResult is the outcome of `avm init`. AlreadyExists means the
// AVM home was already laid out; CreatedPaths is empty in that case.
type InitResult struct {
	Root          string   `json:"root"`
	AlreadyExists bool     `json:"already_exists,omitempty"`
	CreatedPaths  []string `json:"created_paths,omitempty"`
}

// SetupResult is the product-level first-run bootstrap result. It is
// intentionally broader than InitResult: setup owns the user-facing
// onboarding path, while init only lays out the AVM home.
type SetupResult struct {
	Init      InitResult           `json:"init"`
	Runtimes  []SetupRuntimeResult `json:"runtimes,omitempty"`
	NextSteps []string             `json:"next_steps,omitempty"`
}

// SetupRuntimeResult records setup's per-runtime capability import
// attempt. Unavailable runtimes are reported but not treated as fatal.
type SetupRuntimeResult struct {
	Runtime   string                   `json:"runtime"`
	Available bool                     `json:"available"`
	Binary    string                   `json:"binary,omitempty"`
	Version   string                   `json:"version,omitempty"`
	Imported  []ImportCapabilityResult `json:"imported,omitempty"`
	Skipped   []SkippedCapability      `json:"skipped,omitempty"`
	Issues    []string                 `json:"issues,omitempty"`
}

// UninstallResult is the outcome of `avm uninstall` for the AVM home.
// CLI is responsible for removing the binary itself (which lives
// outside the home directory and is process-self-aware via os.Executable).
type UninstallResult struct {
	Root    string `json:"root"`
	Removed bool   `json:"removed,omitempty"`
}
