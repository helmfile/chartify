package chartify

const (
	// EnvVarTempDir is the name of the environment variable that
	// contains the path of the specific directory to be used for generating
	// temporary charts.
	EnvVarTempDir = "CHARTIFY_TEMPDIR"

	// EnvVarDebug is the name of environment variable that
	// is set to a non-empty string whenever the user wants to enable
	// debugging functionality.
	// Currently, the only functionality is to write a `${temporary_chart_name}.json`
	// file that contains all the parameters used to generate the random temporary chart name.
	EnvVarDebug = "CHARTIFY_DEBUG"
)
