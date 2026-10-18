package media

import "fmt"

// FormatDuration formats a play time or song length as "m:ss", or as
// "h:mm:ss" from one hour on. Negative values are shown as 0:00.
func FormatDuration(secs int) string {
	secs = max(secs, 0)
	if secs >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", secs/3600, secs%3600/60, secs%60)
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}
