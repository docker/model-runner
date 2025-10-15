--- a/readline/readline.go
+++ b/readline/readline.go
@@ -155,7 +155,11 @@
 		case CharNull:
 			continue
 		case CharEsc:
 			esc = true
 		case CharInterrupt:
-			return "", ErrInterrupt
+			// On Ctrl-C, cancel the current input line, print a newline,
+			// and return an empty string to signal to the caller to
+			// continue the application but stop the current chat interaction.
+			// This prevents exiting the application.
+			fmt.Println() // Move to a new line after the interrupt
+			return "", nil // Signal cancellation without exiting
 		case CharPrev:
 			i.historyPrev(buf, &currentLineBuf)
 		case CharNext: