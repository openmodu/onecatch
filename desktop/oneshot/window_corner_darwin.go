//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore -framework WebKit

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import <WebKit/WebKit.h>
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

// Canvas colour mirrored from frontend tokens (--acp-canvas): light #F5F5F0,
// dark #1C1C1C. Resolved against the window's effective appearance at apply
// time — WebKit copies NSColor values into plain RGBA on assignment, so a
// dynamic NSColor would freeze at whatever appearance was active on first set.
static NSColor *oneshotCanvasColor(NSAppearance *appearance) {
	NSAppearanceName match = [appearance bestMatchFromAppearancesWithNames:@[ NSAppearanceNameAqua, NSAppearanceNameDarkAqua ]];
	if ([match isEqualToString:NSAppearanceNameDarkAqua]) {
		return [NSColor colorWithSRGBRed:0x1C / 255.0 green:0x1C / 255.0 blue:0x1C / 255.0 alpha:1.0];
	}
	return [NSColor colorWithSRGBRed:0xF5 / 255.0 green:0xF5 / 255.0 blue:0xF0 / 255.0 alpha:1.0];
}

static WKWebView *oneshotFindWebView(NSView *view) {
	if ([view isKindOfClass:[WKWebView class]]) {
		return (WKWebView *)view;
	}
	for (NSView *child in view.subviews) {
		WKWebView *found = oneshotFindWebView(child);
		if (found != nil) {
			return found;
		}
	}
	return nil;
}

// Live-resize exposes backing layers before WebKit repaints. Three layers can
// show through and every one must match the app canvas: the NSWindow
// background, WKWebView's fill for not-yet-rendered regions (the private
// "backgroundColor" property — the one that actually paints the resize gap),
// and the overscroll/under-page colour.
static void oneshotMatchBackgroundToCanvas(NSWindow *window) {
	NSColor *canvas = oneshotCanvasColor(window.effectiveAppearance);
	window.backgroundColor = canvas;
	// WebKit paints the page from its (asynchronous) WebContent process, so
	// during zoom/resize the enlarged region is transparent until the next
	// content frame arrives — whatever sits behind the window shows through.
	// A CALayer background stretches synchronously with the animation, so the
	// frame view fills that gap with the canvas colour instead.
	NSView *frame = window.contentView.superview;
	frame.wantsLayer = YES;
	frame.layer.backgroundColor = canvas.CGColor;
	WKWebView *webView = oneshotFindWebView(window.contentView);
	if (webView == nil) {
		return;
	}
	@try {
		// Stop WebKit from compositing its own opaque white base layer — it
		// stretches synchronously during live resize and would cover the
		// canvas-coloured frame layer below. The page CSS paints an opaque
		// canvas background, so nothing user-visible becomes translucent.
		[webView setValue:@NO forKey:@"drawsBackground"];
		[webView setValue:canvas forKey:@"backgroundColor"];
	} @catch (NSException *exception) {
		// Private WebKit properties; ignore if a future SDK removes them.
	}
	if (@available(macOS 12.0, *)) {
		webView.underPageBackgroundColor = canvas;
	}
}

@interface OneshotAppearanceObserver : NSObject
@property(nonatomic, assign) NSWindow *window;
@end

@implementation OneshotAppearanceObserver
- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary *)change context:(void *)context {
	if ([keyPath isEqualToString:@"effectiveAppearance"]) {
		oneshotMatchBackgroundToCanvas(self.window);
	}
}
@end

static char oneshotAppearanceObserverKey;

static void oneshotInstallAppearanceObserver(NSWindow *window) {
	if (objc_getAssociatedObject(window, &oneshotAppearanceObserverKey) != nil) {
		return;
	}
	OneshotAppearanceObserver *observer = [[OneshotAppearanceObserver alloc] init];
	observer.window = window;
	[window addObserver:observer forKeyPath:@"effectiveAppearance" options:NSKeyValueObservingOptionNew context:NULL];
	objc_setAssociatedObject(window, &oneshotAppearanceObserverKey, observer, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	[observer release];
}

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

static BOOL oneshotAcceptsFirstMouse(id self, SEL _cmd, NSEvent *event) {
	return YES;
}

// Wails' WebviewWindow lets the window itself become key/main/first responder,
// but never overrides NSView's default acceptsFirstMouse (NO). AppKit's default
// behaviour is: the first click on an unfocused window only activates it; the
// click is not delivered to whatever is underneath. Since this window's entire
// content is one WKWebView, that means every input and every button needs a
// second click after the app regained focus (Cmd-Tab, clicking the title bar,
// switching from another app) — the very first click just wakes the window up.
// This is a known upstream gap (wails GitHub #3783). Overriding the method at
// the WKWebView class level makes the activating click also count as a real
// one. class_addMethod is a documented no-op if the class already implements
// the selector, so calling this on every corner-radius pass (window
// maximize/fullscreen toggles) is harmless. Scoped to this process: WKWebView
// is a shared framework class, but this app only ever creates one instance.
static void oneshotEnableClickToFocus(NSWindow *window) {
	WKWebView *webView = oneshotFindWebView(window.contentView);
	if (webView == nil) {
		return;
	}
	class_addMethod([webView class], @selector(acceptsFirstMouse:), (IMP)oneshotAcceptsFirstMouse, "c@:@");
}

static void oneshotSetWindowCornerRadius(void *handle, double radius) {
	NSWindow *window = (__bridge NSWindow *)handle;
	if (window == nil || window.contentView == nil) {
		return;
	}
	oneshotInstallTitlebarDoubleClick(window);
	oneshotInstallAppearanceObserver(window);
	oneshotEnableClickToFocus(window);
	oneshotMatchBackgroundToCanvas(window);

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
