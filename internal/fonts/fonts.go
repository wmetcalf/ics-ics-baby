package fonts

import _ "embed"

// Embedded font data enabling wide Unicode coverage without system dependencies.
//
//go:embed DejaVuSans.ttf
var DejaVuSans []byte

//go:embed NotoSans-Regular.ttf
var NotoSansRegular []byte

//go:embed NotoSans-Bold.ttf
var NotoSansBold []byte

//go:embed EmojiOneColor.otf
var EmojiOneColor []byte
