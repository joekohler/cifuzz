package log

import (
	"github.com/pterm/pterm"
)

const (
	BuildInProgressMsg        string = "Build in progress..."
	BuildInProgressSuccessMsg string = "Build in progress... Done."
	BuildInProgressErrorMsg   string = "Build in progress... Error."

	BundleInProgressMsg        string = "Bundle in progress..."
	BundleInProgressSuccessMsg string = "Bundle in progress... Done."
	BundleInProgressErrorMsg   string = "Bundle in progress... Error."

	ContainerBuildInProgressMsg        string = "Building fuzz container..."
	ContainerBuildInProgressSuccessMsg string = "Building fuzz container... Done."
	ContainerBuildInProgressErrorMsg   string = "Building fuzz container... Error."

	ContainerRunInProgressMsg        string = "Running fuzz container..."
	ContainerRunInProgressSuccessMsg string = "Running fuzz container... Done."
	ContainerRunInProgressErrorMsg   string = "Running fuzz container... Error."
)

func GetPtermErrorStyle() *pterm.Style {
	return &pterm.Style{pterm.FgRed, pterm.Bold}
}

func GetPtermSuccessStyle() *pterm.Style {
	return &pterm.Style{pterm.FgGreen}
}

// Set this, so it can be checked and used in the logging process
// to ensure correct output
var currentProgressSpinner *pterm.SpinnerPrinter

func CreateCurrentProgressSpinner(style *pterm.Style, msg string) {
	if PlainStyle() {
		// do not show a printer when plain style is enabled
		// and only display message
		Info(msg)
		return
	}

	if style != nil {
		currentProgressSpinner.Style = style
		currentProgressSpinner.MessageStyle = style
	}

	// error can be ignored here since pterm doesn't return one
	currentProgressSpinner, _ = pterm.DefaultSpinner.Start(msg)
}

func UpdateCurrentProgressSpinner(msg string) {
	if msg != "" && currentProgressSpinner != nil {
		currentProgressSpinner.UpdateText(msg)
	}
}

func StopCurrentProgressSpinner(style *pterm.Style, msg string) {
	if currentProgressSpinner == nil || PlainStyle() {
		// do not show a printer if it is not set or
		// plain style is enabled and only display message
		Info(msg)
		return
	}

	if style != nil {
		currentProgressSpinner.Style = style
		currentProgressSpinner.MessageStyle = style
	}

	if msg != "" {
		currentProgressSpinner.UpdateText(msg)
	}

	// error can be ignored here since pterm doesn't return one
	currentProgressSpinner.RemoveWhenDone = false
	_ = currentProgressSpinner.Stop()
	currentProgressSpinner = nil
}
