//go:build darwin && cgo

package desktop

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
static char oneshotSidebarMaterialKey;
static char oneshotCanvasBackdropKey;
static char oneshotSidebarBorderKey;
static char oneshotSidebarBridgeKey;
static char oneshotWindowBorderOverlayKey;
static NSString *const oneshotSidebarMessageName = @"oneshotSidebar";
static const CGFloat oneshotSidebarCornerRadius = 16.0;
// The rail is an inset floating panel, not a flush column: it clears the window
// edge on the left, top and bottom, and leaves a gap before the content panel.
static const CGFloat oneshotSidebarInset = 8.0;
static const CGFloat oneshotSidebarGutter = 4.0;

static NSRect oneshotSidebarPanelFrame(NSRect bounds, CGFloat railWidth) {
	CGFloat width = MAX(0.0, MIN(railWidth, NSWidth(bounds)) - oneshotSidebarInset - oneshotSidebarGutter);
	return NSMakeRect(NSMinX(bounds) + oneshotSidebarInset,
	                  NSMinY(bounds) + oneshotSidebarInset,
	                  width,
	                  MAX(0.0, NSHeight(bounds) - oneshotSidebarInset * 2.0));
}

// Canvas colour mirrored from frontend tokens (--acp-canvas): light #F5F5F0,
// dark #1C1C1C. Resolved against the window's effective appearance at apply
// time — WebKit copies NSColor values into plain RGBA on assignment, so a
// dynamic NSColor would freeze at whatever appearance was active on first set.
// AppKit strokes a dark hairline around a stock light window and a light one
// around a dark window. Hidden-titlebar/full-size-content windows lose that
// contrast edge, so resolve an equivalent colour ourselves.
static NSColor *oneshotFrameBorderColor(NSAppearance *appearance) {
	NSAppearanceName match = [appearance bestMatchFromAppearancesWithNames:@[ NSAppearanceNameAqua, NSAppearanceNameDarkAqua ]];
	if ([match isEqualToString:NSAppearanceNameDarkAqua]) {
		return [NSColor colorWithWhite:1.0 alpha:0.22];
	}
	return [NSColor colorWithWhite:0.0 alpha:0.18];
}

static NSColor *oneshotSidebarBorderColor(NSAppearance *appearance) {
	NSAppearanceName match = [appearance bestMatchFromAppearancesWithNames:@[ NSAppearanceNameAqua, NSAppearanceNameDarkAqua ]];
	if ([match isEqualToString:NSAppearanceNameDarkAqua]) {
		return [NSColor colorWithWhite:1.0 alpha:0.22];
	}
	return [NSColor colorWithWhite:0.0 alpha:0.14];
}

static CGFloat oneshotDeviceHairlineWidth(NSWindow *window) {
	CGFloat scale = window.backingScaleFactor;
	return scale > 0.0 ? 1.0 / scale : 1.0;
}

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

// A border placed on the frame layer is underneath AppKit's child-view
// layers. WKWebView therefore covers every straight section and only leaves a
// trace at antialiased corners. Keep the stroke in a sibling above WebKit and
// opt it out of hit-testing so it remains purely visual.
@interface OneshotWindowBorderView : NSView
@end

@implementation OneshotWindowBorderView
- (NSView *)hitTest:(NSPoint)point {
	return nil;
}
@end

static NSView *oneshotInstallWindowBorderOverlay(NSWindow *window) {
	NSView *borderView = objc_getAssociatedObject(window, &oneshotWindowBorderOverlayKey);
	if (borderView != nil) {
		return borderView;
	}

	WKWebView *webView = oneshotFindWebView(window.contentView);
	if (webView == nil || webView.superview == nil) {
		return nil;
	}

	NSView *container = webView.superview;
	OneshotWindowBorderView *overlay = [[OneshotWindowBorderView alloc] initWithFrame:container.bounds];
	overlay.wantsLayer = YES;
	overlay.layer.backgroundColor = [NSColor clearColor].CGColor;
	overlay.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
	[container addSubview:overlay positioned:NSWindowAbove relativeTo:webView];
	objc_setAssociatedObject(window, &oneshotWindowBorderOverlayKey, overlay, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	borderView = overlay;
	[overlay release];
	return borderView;
}

static void oneshotUpdateWindowBorder(NSWindow *window, CGFloat radius) {
	NSView *borderView = oneshotInstallWindowBorderOverlay(window);
	if (borderView.layer == nil) {
		return;
	}
	borderView.frame = borderView.superview.bounds;
	borderView.layer.contentsScale = window.backingScaleFactor;
	borderView.layer.cornerRadius = radius;
	borderView.layer.cornerCurve = kCACornerCurveContinuous;
	borderView.layer.borderWidth = radius > 0.0 ? oneshotDeviceHairlineWidth(window) : 0.0;
	borderView.layer.borderColor = oneshotFrameBorderColor(window.effectiveAppearance).CGColor;
}

@interface OneshotSidebarBridge : NSObject <WKScriptMessageHandler>
@property(nonatomic, assign) NSWindow *window;
@property(nonatomic, assign) NSVisualEffectView *effectView;
@property(nonatomic, assign) NSView *canvasView;
@property(nonatomic, assign) NSView *borderView;
@end

@implementation OneshotSidebarBridge
- (void)userContentController:(WKUserContentController *)userContentController didReceiveScriptMessage:(WKScriptMessage *)message {
	if (![message.name isEqualToString:oneshotSidebarMessageName]) {
		return;
	}

	id body = message.body;
	NSNumber *width = nil;
	NSString *theme = nil;
	if ([body isKindOfClass:[NSDictionary class]]) {
		id candidateWidth = [(NSDictionary *)body objectForKey:@"width"];
		if ([candidateWidth isKindOfClass:[NSNumber class]]) {
			width = candidateWidth;
		}
		id candidateTheme = [(NSDictionary *)body objectForKey:@"theme"];
		if ([candidateTheme isKindOfClass:[NSString class]]) {
			theme = candidateTheme;
		}
	} else if ([body isKindOfClass:[NSNumber class]]) {
		width = body;
	}

	if (width != nil && self.effectView.superview != nil) {
		NSRect bounds = self.effectView.superview.bounds;
		self.effectView.frame = oneshotSidebarPanelFrame(bounds, width.doubleValue);
		self.canvasView.frame = bounds;
		self.borderView.frame = self.effectView.frame;
	}

	// A pinned web theme must also pin AppKit; otherwise a dark sidebar
	// material can sit beside a light CSS canvas (or vice versa). nil restores
	// the normal system-following appearance and keeps the existing KVO path.
	if ([theme isEqualToString:@"light"]) {
		self.window.appearance = [NSAppearance appearanceNamed:NSAppearanceNameAqua];
	} else if ([theme isEqualToString:@"dark"]) {
		self.window.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
	} else if ([theme isEqualToString:@"system"]) {
		self.window.appearance = nil;
	}
}
@end

// The WebView remains full-window so Wails keeps owning input and layout. A
// native effect view sits directly below it and covers only the sidebar. CSS
// makes that one page region transparent; the main column still paints the
// opaque canvas, so live resize never exposes the desktop there.
static void oneshotInstallSidebarMaterial(NSWindow *window) {
	if (objc_getAssociatedObject(window, &oneshotSidebarMaterialKey) != nil) {
		return;
	}
	WKWebView *webView = oneshotFindWebView(window.contentView);
	if (webView == nil || webView.superview == nil) {
		return;
	}

	NSView *container = webView.superview;
	NSView *canvasView = [[NSView alloc] initWithFrame:container.bounds];
	canvasView.wantsLayer = YES;
	canvasView.layer.backgroundColor = oneshotCanvasColor(window.effectiveAppearance).CGColor;
	canvasView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
	NSVisualEffectView *effectView = [[NSVisualEffectView alloc]
		initWithFrame:oneshotSidebarPanelFrame(container.bounds, 216.0)];
	effectView.material = NSVisualEffectMaterialSidebar;
	effectView.blendingMode = NSVisualEffectBlendingModeBehindWindow;
	effectView.state = NSVisualEffectStateFollowsWindowActiveState;
	effectView.emphasized = NO;
	effectView.autoresizingMask = NSViewHeightSizable | NSViewMaxXMargin;
	effectView.wantsLayer = YES;
	effectView.layer.cornerRadius = oneshotSidebarCornerRadius;
	effectView.layer.cornerCurve = kCACornerCurveContinuous;
	effectView.layer.masksToBounds = YES;
	OneshotWindowBorderView *borderView = [[OneshotWindowBorderView alloc] initWithFrame:effectView.frame];
	borderView.wantsLayer = YES;
	borderView.layer.backgroundColor = [NSColor clearColor].CGColor;
	borderView.layer.cornerRadius = oneshotSidebarCornerRadius;
	borderView.layer.cornerCurve = kCACornerCurveContinuous;
	borderView.layer.borderWidth = oneshotDeviceHairlineWidth(window);
	borderView.layer.borderColor = oneshotSidebarBorderColor(window.effectiveAppearance).CGColor;
	borderView.autoresizingMask = NSViewHeightSizable | NSViewMaxXMargin;
	[container addSubview:canvasView positioned:NSWindowBelow relativeTo:webView];
	[container addSubview:effectView positioned:NSWindowBelow relativeTo:webView];
	[container addSubview:borderView positioned:NSWindowAbove relativeTo:webView];

	OneshotSidebarBridge *bridge = [[OneshotSidebarBridge alloc] init];
	bridge.window = window;
	bridge.effectView = effectView;
	bridge.canvasView = canvasView;
	bridge.borderView = borderView;
	[webView.configuration.userContentController addScriptMessageHandler:bridge name:oneshotSidebarMessageName];

	objc_setAssociatedObject(window, &oneshotSidebarMaterialKey, effectView, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	objc_setAssociatedObject(window, &oneshotCanvasBackdropKey, canvasView, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	objc_setAssociatedObject(window, &oneshotSidebarBorderKey, borderView, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	objc_setAssociatedObject(window, &oneshotSidebarBridgeKey, bridge, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	[canvasView release];
	[effectView release];
	[borderView release];
	[bridge release];

	// Mark the native runtime only after the effect exists. Browser previews
	// retain their solid sidebar fallback. The message also catches an initial
	// React width/theme update that may have run before this handler was added.
	NSString *script =
		@"document.documentElement.dataset.nativeSidebarMaterial='true';"
		 "window.webkit.messageHandlers.oneshotSidebar.postMessage({"
		 "width:document.querySelector('.sidebar')?.getBoundingClientRect().width||216,"
		 "theme:document.documentElement.dataset.theme||'system'"
		 "});";
	[webView evaluateJavaScript:script completionHandler:nil];

	// Behind-window materials need a non-opaque window even though the main
	// column keeps its own fully painted native backing view.
	window.opaque = NO;
	window.backgroundColor = [NSColor clearColor];
}

// Live-resize exposes backing layers before WebKit repaints. The window,
// frame, WebView base and under-page colour must stay clear so the native
// sidebar material remains visible. The sibling canvas view installed above
// is the synchronous opaque fallback for the main column.
static void oneshotMatchBackgroundToCanvas(NSWindow *window) {
	NSColor *canvas = oneshotCanvasColor(window.effectiveAppearance);
	window.backgroundColor = [NSColor clearColor];
	NSView *frame = window.contentView.superview;
	frame.wantsLayer = YES;
	frame.layer.backgroundColor = [NSColor clearColor].CGColor;
	NSView *canvasView = objc_getAssociatedObject(window, &oneshotCanvasBackdropKey);
	if (canvasView.layer != nil) {
		canvasView.layer.backgroundColor = canvas.CGColor;
	}
	WKWebView *webView = oneshotFindWebView(window.contentView);
	if (webView == nil) {
		return;
	}
	@try {
		// Stop WebKit from compositing its own opaque white base layer over the
		// native material and canvas views. Page CSS still paints the main area.
		[webView setValue:@NO forKey:@"drawsBackground"];
		[webView setValue:[NSColor clearColor] forKey:@"backgroundColor"];
	} @catch (NSException *exception) {
		// Private WebKit properties; ignore if a future SDK removes them.
	}
	if (@available(macOS 12.0, *)) {
		webView.underPageBackgroundColor = [NSColor clearColor];
	}
}

@interface OneshotAppearanceObserver : NSObject
@property(nonatomic, assign) NSWindow *window;
@end

@implementation OneshotAppearanceObserver
- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary *)change context:(void *)context {
	if ([keyPath isEqualToString:@"effectiveAppearance"]) {
		oneshotMatchBackgroundToCanvas(self.window);
		NSView *borderView = objc_getAssociatedObject(self.window, &oneshotWindowBorderOverlayKey);
		if (borderView.layer != nil && borderView.layer.borderWidth > 0.0) {
			borderView.layer.borderColor = oneshotFrameBorderColor(self.window.effectiveAppearance).CGColor;
		}
		NSView *sidebarBorder = objc_getAssociatedObject(self.window, &oneshotSidebarBorderKey);
		if (sidebarBorder.layer != nil) {
			sidebarBorder.layer.borderColor = oneshotSidebarBorderColor(self.window.effectiveAppearance).CGColor;
		}
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
	// Without this, every single click in the window costs the user a second one.
	// delaysPrimaryMouseButtonEvents defaults to YES, which holds left-button
	// events back from the view hierarchy until the recognizer decides whether it
	// matched. Requiring two clicks to match means a lone click is held for the
	// double-click interval and then replayed, which the WKWebView underneath
	// sees as a click that only moves focus — so the control fires on the *next*
	// click instead. gestureRecognizerShouldBegin only limits where the gesture
	// may begin (the titlebar strip); it does not stop the delay from applying to
	// the whole content view. Setting NO lets the recognizer still observe the
	// titlebar double-click without ever intercepting ordinary clicks.
	recognizer.delaysPrimaryMouseButtonEvents = NO;
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
	oneshotInstallSidebarMaterial(window);

	NSView *frame = window.contentView.superview;
	frame.wantsLayer = YES;
	frame.layer.cornerRadius = radius;
	frame.layer.cornerCurve = kCACornerCurveContinuous;
	frame.layer.masksToBounds = radius > 0;
	frame.layer.borderWidth = 0.0;
	oneshotUpdateWindowBorder(window, radius);
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
