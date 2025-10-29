package sandbox

import (
	"maps"
	"slices"
	"testing"
)

// configurationTesting supports TestConfigurationParsing.
const configurationTesting = `   (WithDesktopLimit)
Ignore this
  (WithDieOnUnhandledException)
;; Comments
(WithDisplaySettingsLimit)
		(WithExitWindowsLimit)
  (WithGlobalAtomsLimit) (WithHandlesLimit)
(WithDisableOutgoingNetworking)

   (IgnoreMeToo!)
(WithReadClipboardLimit)

	(WithSystemParametersLimit)
(WithWriteClipboardLimit)
`

// TestConfigurationParsing does some basic configuration parsing testing.
func TestConfigurationParsing(t *testing.T) {
	tokens := limitTokenMatcher.FindAllString(configurationTesting, -1)
	slices.Sort(tokens)
	keys := slices.Collect(maps.Keys(limitTokenToGenerator))
	slices.Sort(keys)
	if !slices.Equal(tokens, keys) {
		t.Error("parsed configuration tokens don't match known values")
	}
}
