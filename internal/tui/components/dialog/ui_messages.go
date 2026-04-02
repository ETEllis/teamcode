package dialog

// ShowHelpDialogMsg opens the keyboard shortcut help overlay.
type ShowHelpDialogMsg struct{}

// ShowSessionDialogMsg opens the session picker overlay.
type ShowSessionDialogMsg struct{}

// ShowThemeDialogMsg opens the theme picker overlay.
type ShowThemeDialogMsg struct{}

// ShowQuitDialogMsg opens the quit confirmation overlay.
type ShowQuitDialogMsg struct{}

// ShowAgencyDialogMsg opens the Agency status overlay.
type ShowAgencyDialogMsg struct{}

// CloseAgencyDialogMsg closes the Agency status overlay.
type CloseAgencyDialogMsg struct{}

// BootAgencyOfficeMsg starts or reconciles the Agency office runtime.
type BootAgencyOfficeMsg struct {
	Constitution string
}

// StopAgencyOfficeMsg stops the Agency office runtime.
type StopAgencyOfficeMsg struct{}

// StartAgencyGenesisMsg records a genesis brief using the configured Agency service.
type StartAgencyGenesisMsg struct {
	Intent       string
	Constitution string
}
