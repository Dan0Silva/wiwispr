package typer

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Linux input event and uinput constants (from <linux/input.h> and <linux/uinput.h>)
const (
	evSyn = 0x00
	evKey = 0x01

	synReport = 0

	uinputMaxNameSize = 80
	uiSetEvBit        = 0x40045564
	uiSetKeyBit       = 0x40045565
	uiDevCreate       = 0x5501
	uiDevDestroy      = 0x5502
)

// Linux evdev keycodes (physical key positions on standard US / PC105 / ABNT2 keyboard)
const (
	KeyEsc        = 1
	Key1          = 2
	Key2          = 3
	Key3          = 4
	Key4          = 5
	Key5          = 6
	Key6          = 7
	Key7          = 8
	Key8          = 9
	Key9          = 10
	Key0          = 11
	KeyMinus      = 12
	KeyEqual      = 13
	KeyBackspace  = 14
	KeyTab        = 15
	KeyQ          = 16
	KeyW          = 17
	KeyE          = 18
	KeyR          = 19
	KeyT          = 20
	KeyY          = 21
	KeyU          = 22
	KeyI          = 23
	KeyO          = 24
	KeyP          = 25
	KeyLeftBrace  = 26
	KeyRightBrace = 27
	KeyEnter      = 28
	KeyLeftCtrl   = 29
	KeyA          = 30
	KeyS          = 31
	KeyD          = 32
	KeyF          = 33
	KeyG          = 34
	KeyH          = 35
	KeyJ          = 36
	KeyK          = 37
	KeyL          = 38
	KeySemicolon  = 39
	KeyApostrophe = 40
	KeyGrave      = 41
	KeyLeftShift  = 42
	KeyBackslash  = 43
	KeyZ          = 44
	KeyX          = 45
	KeyC          = 46
	KeyV          = 47
	KeyB          = 48
	KeyN          = 49
	KeyM          = 50
	KeyComma      = 51
	KeyDot        = 52
	KeySlash      = 53
	KeyRightShift = 54
	KeyLeftAlt    = 56
	KeySpace      = 57
	KeyRightAlt   = 100 // AltGr
	KeyRo         = 89  // ABNT2 '/' and '?' key next to Right Shift
)

// KeyAction represents a single physical key tap (with optional Shift or AltGr modifier).
type KeyAction struct {
	Code  uint16
	Shift bool
	AltGr bool
}

// uinputUserDev mirrors struct uinput_user_dev in <linux/uinput.h>.
type uinputUserDev struct {
	Name       [uinputMaxNameSize]byte
	IDBusType  uint16
	IDVendor   uint16
	IDProduct  uint16
	IDVersion  uint16
	FFEffects  uint32
	AbsMax     [64]int32
	AbsMin     [64]int32
	AbsFuzz    [64]int32
	AbsFlat    [64]int32
}

// VirtualKeyboard wraps a Linux /dev/uinput virtual keyboard device.
type VirtualKeyboard struct {
	fd *os.File
}

// NewVirtualKeyboard opens /dev/uinput and registers a virtual keyboard so KDE Wayland
// treats our simulated keystrokes identically to a hardware USB keyboard.
func NewVirtualKeyboard() (*VirtualKeyboard, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_WRONLY|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, fmt.Errorf("open /dev/uinput: %w", err)
	}

	rawFd := f.Fd()
	if err := ioctl(rawFd, uiSetEvBit, uintptr(evKey)); err != nil {
		f.Close()
		return nil, fmt.Errorf("UI_SET_EVBIT(EV_KEY): %w", err)
	}
	if err := ioctl(rawFd, uiSetEvBit, uintptr(evSyn)); err != nil {
		f.Close()
		return nil, fmt.Errorf("UI_SET_EVBIT(EV_SYN): %w", err)
	}

	// Enable standard keycodes (1..127)
	for k := uintptr(1); k < 128; k++ {
		_ = ioctl(rawFd, uiSetKeyBit, k)
	}

	var dev uinputUserDev
	copy(dev.Name[:], []byte("wiwispr-virtual-keyboard"))
	dev.IDBusType = 0x03 // BUS_USB
	dev.IDVendor = 0x1209
	dev.IDProduct = 0x7777
	dev.IDVersion = 1

	devBytes := (*[unsafe.Sizeof(dev)]byte)(unsafe.Pointer(&dev))[:]
	if _, err := f.Write(devBytes); err != nil {
		f.Close()
		return nil, fmt.Errorf("write uinput_user_dev: %w", err)
	}

	if err := ioctl(rawFd, uiDevCreate, 0); err != nil {
		f.Close()
		return nil, fmt.Errorf("UI_DEV_CREATE: %w", err)
	}

	// Give KWin Wayland compositor ~120ms to discover and bind the new virtual input device
	time.Sleep(120 * time.Millisecond)

	return &VirtualKeyboard{fd: f}, nil
}

// Close destroys the virtual keyboard device.
func (vk *VirtualKeyboard) Close() {
	if vk == nil || vk.fd == nil {
		return
	}
	time.Sleep(35 * time.Millisecond)
	_ = ioctl(vk.fd.Fd(), uiDevDestroy, 0)
	_ = vk.fd.Close()
	vk.fd = nil
}

func (vk *VirtualKeyboard) emit(evType, code uint16, value int32) error {
	// struct input_event on 64-bit Linux is 24 bytes:
	//   sec  int64 (8)
	//   usec int64 (8)
	//   type uint16 (2)
	//   code uint16 (2)
	//   val  int32 (4)
	var buf [24]byte
	binary.LittleEndian.PutUint16(buf[16:18], evType)
	binary.LittleEndian.PutUint16(buf[18:20], code)
	binary.LittleEndian.PutUint32(buf[20:24], uint32(value))
	_, err := vk.fd.Write(buf[:])
	return err
}

func (vk *VirtualKeyboard) sync() error {
	return vk.emit(evSyn, synReport, 0)
}

// Tap executes a single KeyAction (pressing Shift/AltGr if needed, pressing the key, releasing).
func (vk *VirtualKeyboard) Tap(act KeyAction) error {
	if act.Shift {
		if err := vk.emit(evKey, KeyLeftShift, 1); err != nil {
			return err
		}
		_ = vk.sync()
		time.Sleep(2 * time.Millisecond)
	}
	if act.AltGr {
		if err := vk.emit(evKey, KeyRightAlt, 1); err != nil {
			return err
		}
		_ = vk.sync()
		time.Sleep(2 * time.Millisecond)
	}

	if err := vk.emit(evKey, act.Code, 1); err != nil {
		return err
	}
	_ = vk.sync()
	time.Sleep(3 * time.Millisecond)

	if err := vk.emit(evKey, act.Code, 0); err != nil {
		return err
	}
	_ = vk.sync()

	if act.AltGr {
		time.Sleep(2 * time.Millisecond)
		_ = vk.emit(evKey, KeyRightAlt, 0)
		_ = vk.sync()
	}
	if act.Shift {
		time.Sleep(2 * time.Millisecond)
		_ = vk.emit(evKey, KeyLeftShift, 0)
		_ = vk.sync()
	}
	return nil
}

// TypeText simulates typing text character-by-character using /dev/uinput with a typewriter delay.
// If /dev/uinput is unavailable, it falls back to wl-copy.
func TypeText(text string, delay time.Duration, layout string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	vk, err := NewVirtualKeyboard()
	if err != nil {
		// Fallback to clipboard copy if /dev/uinput fails
		if wlPath, lookErr := exec.LookPath("wl-copy"); lookErr == nil {
			cmd := exec.Command(wlPath, text)
			_ = cmd.Run()
		}
		return fmt.Errorf("virtual keyboard unavailable (%w); text copied to clipboard via wl-copy", err)
	}
	defer vk.Close()

	actions := StringToKeyActions(text, layout)
	for _, act := range actions {
		if err := vk.Tap(act); err != nil {
			return err
		}
		if delay > 0 {
			time.Sleep(delay)
		}
	}
	return nil
}

// StringToKeyActions converts a UTF-8 Portuguese string into a sequence of physical key actions
// for either "us-intl" (US International / alt-intl) or "abnt2" (Brazilian ABNT2) layouts.
func StringToKeyActions(text string, layout string) []KeyAction {
	isABNT2 := strings.EqualFold(layout, "abnt2") || strings.EqualFold(layout, "br")
	var out []KeyAction
	for _, r := range text {
		if isABNT2 {
			out = append(out, RuneToABNT2(r)...)
		} else {
			out = append(out, RuneToUSIntl(r)...)
		}
	}
	return out
}

// RuneToUSIntl maps a rune to key actions on a US-International (us-alt-intl / us-intl) layout.
// Note: On US-Intl, apostrophe ('), quote ("), grave (`), tilde (~), and caret (^) are dead keys!
// Therefore:
//   - Accented letters like 'á' = dead_acute (KeyApostrophe) + 'a' (KeyA)
//   - Literal apostrophe (') = dead_acute (KeyApostrophe) + Space (KeySpace)
func RuneToUSIntl(r rune) []KeyAction {
	// Basic lowercase a-z
	if r >= 'a' && r <= 'z' {
		if code, ok := asciiLetterCode(r); ok {
			return []KeyAction{{Code: code}}
		}
	}
	// Basic uppercase A-Z
	if r >= 'A' && r <= 'Z' {
		if code, ok := asciiLetterCode(r + 32); ok {
			return []KeyAction{{Code: code, Shift: true}}
		}
	}
	// Digits 0-9
	if r >= '0' && r <= '9' {
		if code, ok := digitCode(r); ok {
			return []KeyAction{{Code: code}}
		}
	}

	// Portuguese accented characters on US-International dead-key layout:
	switch r {
	// Acute accent (dead key: KeyApostrophe)
	case 'á':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyA}}
	case 'Á':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyA, Shift: true}}
	case 'é':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyE}}
	case 'É':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyE, Shift: true}}
	case 'í':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyI}}
	case 'Í':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyI, Shift: true}}
	case 'ó':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyO}}
	case 'Ó':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyO, Shift: true}}
	case 'ú':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyU}}
	case 'Ú':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyU, Shift: true}}

	// Cedilla (on US-Intl: dead acute + c -> ç, or AltGr + comma)
	case 'ç':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyC}}
	case 'Ç':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyC, Shift: true}}

	// Tilde (dead key: Shift + KeyGrave)
	case 'ã':
		return []KeyAction{{Code: KeyGrave, Shift: true}, {Code: KeyA}}
	case 'Ã':
		return []KeyAction{{Code: KeyGrave, Shift: true}, {Code: KeyA, Shift: true}}
	case 'õ':
		return []KeyAction{{Code: KeyGrave, Shift: true}, {Code: KeyO}}
	case 'Õ':
		return []KeyAction{{Code: KeyGrave, Shift: true}, {Code: KeyO, Shift: true}}
	case 'ñ':
		return []KeyAction{{Code: KeyGrave, Shift: true}, {Code: KeyN}}
	case 'Ñ':
		return []KeyAction{{Code: KeyGrave, Shift: true}, {Code: KeyN, Shift: true}}

	// Circumflex (dead key: Shift + Key6)
	case 'â':
		return []KeyAction{{Code: Key6, Shift: true}, {Code: KeyA}}
	case 'Â':
		return []KeyAction{{Code: Key6, Shift: true}, {Code: KeyA, Shift: true}}
	case 'ê':
		return []KeyAction{{Code: Key6, Shift: true}, {Code: KeyE}}
	case 'Ê':
		return []KeyAction{{Code: Key6, Shift: true}, {Code: KeyE, Shift: true}}
	case 'ô':
		return []KeyAction{{Code: Key6, Shift: true}, {Code: KeyO}}
	case 'Ô':
		return []KeyAction{{Code: Key6, Shift: true}, {Code: KeyO, Shift: true}}

	// Grave accent (dead key: KeyGrave)
	case 'à':
		return []KeyAction{{Code: KeyGrave}, {Code: KeyA}}
	case 'À':
		return []KeyAction{{Code: KeyGrave}, {Code: KeyA, Shift: true}}

	// Diaeresis / Umlaut (dead key: Shift + KeyApostrophe)
	case 'ü':
		return []KeyAction{{Code: KeyApostrophe, Shift: true}, {Code: KeyU}}
	case 'Ü':
		return []KeyAction{{Code: KeyApostrophe, Shift: true}, {Code: KeyU, Shift: true}}

	// Whitespace & Punctuation (handling dead keys on US-Intl by following with Space)
	case ' ':
		return []KeyAction{{Code: KeySpace}}
	case '\n':
		return []KeyAction{{Code: KeyEnter}}
	case '\t':
		return []KeyAction{{Code: KeyTab}}
	case '.':
		return []KeyAction{{Code: KeyDot}}
	case ',':
		return []KeyAction{{Code: KeyComma}}
	case ';':
		return []KeyAction{{Code: KeySemicolon}}
	case ':':
		return []KeyAction{{Code: KeySemicolon, Shift: true}}
	case '!':
		return []KeyAction{{Code: Key1, Shift: true}}
	case '?':
		return []KeyAction{{Code: KeySlash, Shift: true}}
	case '-':
		return []KeyAction{{Code: KeyMinus}}
	case '_':
		return []KeyAction{{Code: KeyMinus, Shift: true}}
	case '(':
		return []KeyAction{{Code: Key9, Shift: true}}
	case ')':
		return []KeyAction{{Code: Key0, Shift: true}}
	case '[':
		return []KeyAction{{Code: KeyLeftBrace}}
	case ']':
		return []KeyAction{{Code: KeyRightBrace}}
	case '/':
		return []KeyAction{{Code: KeySlash}}
	case '\'':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeySpace}}
	case '"':
		return []KeyAction{{Code: KeyApostrophe, Shift: true}, {Code: KeySpace}}
	case '~':
		return []KeyAction{{Code: KeyGrave, Shift: true}, {Code: KeySpace}}
	case '^':
		return []KeyAction{{Code: Key6, Shift: true}, {Code: KeySpace}}
	case '`':
		return []KeyAction{{Code: KeyGrave}, {Code: KeySpace}}
	}

	return nil
}

// RuneToABNT2 maps a rune to key actions on a Brazilian ABNT2 ("br") keyboard layout.
func RuneToABNT2(r rune) []KeyAction {
	if r >= 'a' && r <= 'z' {
		if code, ok := asciiLetterCode(r); ok {
			return []KeyAction{{Code: code}}
		}
	}
	if r >= 'A' && r <= 'Z' {
		if code, ok := asciiLetterCode(r + 32); ok {
			return []KeyAction{{Code: code, Shift: true}}
		}
	}
	if r >= '0' && r <= '9' {
		if code, ok := digitCode(r); ok {
			return []KeyAction{{Code: code}}
		}
	}

	// On ABNT2:
	//   KeySemicolon (39) is native 'ç'
	//   KeyLeftBrace (26) is dead_acute (´) and Shift+KeyLeftBrace is dead_grave (`)
	//   KeyApostrophe (40) is dead_tilde (~) and Shift+KeyApostrophe is dead_circumflex (^)
	switch r {
	case 'ç':
		return []KeyAction{{Code: KeySemicolon}}
	case 'Ç':
		return []KeyAction{{Code: KeySemicolon, Shift: true}}

	case 'á':
		return []KeyAction{{Code: KeyLeftBrace}, {Code: KeyA}}
	case 'Á':
		return []KeyAction{{Code: KeyLeftBrace}, {Code: KeyA, Shift: true}}
	case 'é':
		return []KeyAction{{Code: KeyLeftBrace}, {Code: KeyE}}
	case 'É':
		return []KeyAction{{Code: KeyLeftBrace}, {Code: KeyE, Shift: true}}
	case 'í':
		return []KeyAction{{Code: KeyLeftBrace}, {Code: KeyI}}
	case 'Í':
		return []KeyAction{{Code: KeyLeftBrace}, {Code: KeyI, Shift: true}}
	case 'ó':
		return []KeyAction{{Code: KeyLeftBrace}, {Code: KeyO}}
	case 'Ó':
		return []KeyAction{{Code: KeyLeftBrace}, {Code: KeyO, Shift: true}}
	case 'ú':
		return []KeyAction{{Code: KeyLeftBrace}, {Code: KeyU}}
	case 'Ú':
		return []KeyAction{{Code: KeyLeftBrace}, {Code: KeyU, Shift: true}}

	case 'ã':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyA}}
	case 'Ã':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyA, Shift: true}}
	case 'õ':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyO}}
	case 'Õ':
		return []KeyAction{{Code: KeyApostrophe}, {Code: KeyO, Shift: true}}

	case 'â':
		return []KeyAction{{Code: KeyApostrophe, Shift: true}, {Code: KeyA}}
	case 'Â':
		return []KeyAction{{Code: KeyApostrophe, Shift: true}, {Code: KeyA, Shift: true}}
	case 'ê':
		return []KeyAction{{Code: KeyApostrophe, Shift: true}, {Code: KeyE}}
	case 'Ê':
		return []KeyAction{{Code: KeyApostrophe, Shift: true}, {Code: KeyE, Shift: true}}
	case 'ô':
		return []KeyAction{{Code: KeyApostrophe, Shift: true}, {Code: KeyO}}
	case 'Ô':
		return []KeyAction{{Code: KeyApostrophe, Shift: true}, {Code: KeyO, Shift: true}}

	case 'à':
		return []KeyAction{{Code: KeyLeftBrace, Shift: true}, {Code: KeyA}}
	case 'À':
		return []KeyAction{{Code: KeyLeftBrace, Shift: true}, {Code: KeyA, Shift: true}}

	case ' ':
		return []KeyAction{{Code: KeySpace}}
	case '\n':
		return []KeyAction{{Code: KeyEnter}}
	case '.':
		return []KeyAction{{Code: KeyDot}}
	case ',':
		return []KeyAction{{Code: KeyComma}}
	case ';':
		return []KeyAction{{Code: KeySlash}}
	case ':':
		return []KeyAction{{Code: KeySlash, Shift: true}}
	case '!':
		return []KeyAction{{Code: Key1, Shift: true}}
	case '?':
		return []KeyAction{{Code: KeyRo, Shift: true}}
	case '/':
		return []KeyAction{{Code: KeyRo}}
	case '-':
		return []KeyAction{{Code: KeyMinus}}
	case '_':
		return []KeyAction{{Code: KeyMinus, Shift: true}}
	case '\'':
		return []KeyAction{{Code: KeyGrave}}
	case '"':
		return []KeyAction{{Code: KeyGrave, Shift: true}}
	}

	return RuneToUSIntl(r)
}

func asciiLetterCode(r rune) (uint16, bool) {
	switch r {
	case 'a':
		return KeyA, true
	case 'b':
		return KeyB, true
	case 'c':
		return KeyC, true
	case 'd':
		return KeyD, true
	case 'e':
		return KeyE, true
	case 'f':
		return KeyF, true
	case 'g':
		return KeyG, true
	case 'h':
		return KeyH, true
	case 'i':
		return KeyI, true
	case 'j':
		return KeyJ, true
	case 'k':
		return KeyK, true
	case 'l':
		return KeyL, true
	case 'm':
		return KeyM, true
	case 'n':
		return KeyN, true
	case 'o':
		return KeyO, true
	case 'p':
		return KeyP, true
	case 'q':
		return KeyQ, true
	case 'r':
		return KeyR, true
	case 's':
		return KeyS, true
	case 't':
		return KeyT, true
	case 'u':
		return KeyU, true
	case 'v':
		return KeyV, true
	case 'w':
		return KeyW, true
	case 'x':
		return KeyX, true
	case 'y':
		return KeyY, true
	case 'z':
		return KeyZ, true
	default:
		return 0, false
	}
}

func digitCode(r rune) (uint16, bool) {
	switch r {
	case '1':
		return Key1, true
	case '2':
		return Key2, true
	case '3':
		return Key3, true
	case '4':
		return Key4, true
	case '5':
		return Key5, true
	case '6':
		return Key6, true
	case '7':
		return Key7, true
	case '8':
		return Key8, true
	case '9':
		return Key9, true
	case '0':
		return Key0, true
	default:
		return 0, false
	}
}

func ioctl(fd uintptr, req uintptr, arg uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg)
	if errno != 0 {
		return errno
	}
	return nil
}
