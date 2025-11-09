package fonts

import _ "embed"

// Embedded font data enabling wide Unicode coverage without system dependencies.
//
//go:embed DejaVuSans.ttf
var DejaVuSans []byte

//go:embed NotoSans-Regular.ttf
var NotoSansRegular []byte

// NotoSans-Bold not used - normal weight is sufficient
// EmojiOneColor not used - color emoji unsupported by opentype renderer
