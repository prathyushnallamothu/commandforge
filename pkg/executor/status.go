package executor

// BackgroundCommandStatus represents the status of a background command
type BackgroundCommandStatus struct {
	ID         string   `json:"id"`
	Command    string   `json:"command"`
	WorkingDir string   `json:"working_dir"`
	Running    bool     `json:"running"`
	ExitCode   int      `json:"exit_code"`
	Output     string   `json:"output"`
	Error      string   `json:"error"`
	OutputList []string `json:"output_list"`
	ErrorList  []string `json:"error_list"`
	Duration   float64  `json:"duration"`
}

// NewBackgroundCommandStatus creates a new background command status
func NewBackgroundCommandStatus(
	id string,
	command string,
	workingDir string,
	running bool,
	exitCode int,
	output string,
	error string,
	outputList []string,
	errorList []string,
	duration float64,
) *BackgroundCommandStatus {
	return &BackgroundCommandStatus{
		ID:         id,
		Command:    command,
		WorkingDir: workingDir,
		Running:    running,
		ExitCode:   exitCode,
		Output:     output,
		Error:      error,
		OutputList: outputList,
		ErrorList:  errorList,
		Duration:   duration,
	}
}
