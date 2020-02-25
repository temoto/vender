// +build ignore

package framebuffer

/*
#include <linux/fb.h>
#include <sys/mman.h>
*/
import "C"

type fixedScreenInfo C.struct_fb_fix_screeninfo
type variableScreenInfo C.struct_fb_var_screeninfo
type bitField C.struct_fb_bitfield

const (
	getFixedScreenInfo    uintptr = C.FBIOGET_FSCREENINFO
	getVariableScreenInfo uintptr = C.FBIOGET_VSCREENINFO
)
