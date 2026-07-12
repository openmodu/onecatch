//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import <objc/runtime.h>

@interface OneshotTitlebarDoubleClickHandler : NSObject <NSGestureRecognizerDelegate>
@property(nonatomic, assign) NSWindow *window;
@property(nonatomic, assign) CGFloat titlebarHeight;
- (void)handleDoubleClick:(NSClickGestureRecognizer *)recognizer;
@end

@implementation OneshotTitlebarDoubleClickHandler
- (BOOL)gestureRecognizerShouldBegin:(NSGestureRecognizer *)recognizer {
	NSPoint point = [recognizer locationInView:recognizer.view];
	return point.y >= NSHeight(recognizer.view.bounds) - self.titlebarHeight;
}

- (void)handleDoubleClick:(NSClickGestureRecognizer *)recognizer {
	if (recognizer.state == NSGestureRecognizerStateEnded) {
		[self.window zoom:nil];
	}
}
@end

static char oneshotTitlebarDoubleClickKey;

static void oneshotInstallTitlebarDoubleClick(NSWindow *window) {
	if (objc_getAssociatedObject(window, &oneshotTitlebarDoubleClickKey) != nil) {
		return;
	}

	OneshotTitlebarDoubleClickHandler *handler = [[OneshotTitlebarDoubleClickHandler alloc] init];
	handler.window = window;
	handler.titlebarHeight = 80.0;

	NSClickGestureRecognizer *recognizer = [[NSClickGestureRecognizer alloc]
		initWithTarget:handler
		action:@selector(handleDoubleClick:)];
	recognizer.numberOfClicksRequired = 2;
	recognizer.buttonMask = 0x1;
	recognizer.delegate = handler;
	[window.contentView addGestureRecognizer:recognizer];
	objc_setAssociatedObject(window, &oneshotTitlebarDoubleClickKey, handler, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	[recognizer release];
	[handler release];
}

static void oneshotSetWindowCornerRadius(void *handle, double radius) {
	NSWindow *window = (__bridge NSWindow *)handle;
	if (window == nil || window.contentView == nil) {
		return;
	}
	oneshotInstallTitlebarDoubleClick(window);

	NSView *frame = window.contentView.superview;
	frame.wantsLayer = YES;
	frame.layer.cornerRadius = radius;
	frame.layer.cornerCurve = kCACornerCurveContinuous;
	frame.layer.masksToBounds = radius > 0;
	[window invalidateShadow];
}
*/
import "C"

import "unsafe"

func setNativeWindowCornerRadius(window unsafe.Pointer, radius float64) {
	if window == nil {
		return
	}
	C.oneshotSetWindowCornerRadius(window, C.double(radius))
}
