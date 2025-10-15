--- a/readline/readline.go
+++ b/readline/readline.go
@@ -107,6 +107,10 @@
 				buf.MoveLeft()
 			case KeyRight:
 				buf.MoveRight()
+			case 'b': // Option+B on iTerm2 can send ESC [ b or ESC [ 1;3b
+				buf.MoveLeftWord()
+			case 'f': // Option+F on iTerm2 can send ESC [ f or ESC [ 1;3f
+				buf.MoveRightWord()
 			case CharBracketedPaste:
 				var code string
 				for range 3 {