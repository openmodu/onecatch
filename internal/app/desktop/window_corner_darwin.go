//go:build darwin && cgo

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore -framework WebKit

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>

static char onecatchSidebarMaterialKey;
static char onecatchCanvasBackdropKey;
static char onecatchSidebarBorderKey;
static char onecatchSidebarBridgeKey;
static NSString *const onecatchSidebarMessageName = @"onecatchSidebar";
static const CGFloat onecatchSidebarCornerRadius = 16.0;
// The rail is an inset floating panel, not a flush column: it clears the window
// edge on the left, top and bottom, and leaves a gap before the content panel.
static const CGFloat onecatchSidebarInset = 8.0;
static const CGFloat onecatchSidebarGutter = 4.0;

static NSRect onecatchSidebarPanelFrame(NSRect bounds, CGFloat railWidth) {
	CGFloat width = MAX(0.0, MIN(railWidth, NSWidth(bounds)) - onecatchSidebarInset - onecatchSidebarGutter);
	return NSMakeRect(NSMinX(bounds) + onecatchSidebarInset,
	                  NSMinY(bounds) + onecatchSidebarInset,
	                  width,
	                  MAX(0.0, NSHeight(bounds) - onecatchSidebarInset * 2.0));
}

static NSRect onecatchCompactSidebarPanelFrame(NSRect bounds, CGFloat railWidth) {
	CGFloat width = MAX(0.0, MIN(railWidth, NSWidth(bounds)) - onecatchSidebarInset - onecatchSidebarGutter);
	CGFloat outerHeight = MIN(560.0, NSHeight(bounds));
	CGFloat height = MAX(0.0, outerHeight - onecatchSidebarInset * 2.0);
	return NSMakeRect(NSMinX(bounds) + onecatchSidebarInset,
	                  NSMaxY(bounds) - onecatchSidebarInset - height,
	                  width,
	                  height);
}

// Canvas colour mirrored from frontend tokens (--acp-canvas): light #F5F5F0,
// dark #1C1C1C.
// Resolved against the window's effective appearance at apply
// time — WebKit copies NSColor values into plain RGBA on assignment, so a
// dynamic NSColor would freeze at whatever appearance was active on first set.
static NSColor *onecatchSidebarBorderColor(NSAppearance *appearance) {
	NSAppearanceName match = [appearance bestMatchFromAppearancesWithNames:@[ NSAppearanceNameAqua, NSAppearanceNameDarkAqua ]];
	if ([match isEqualToString:NSAppearanceNameDarkAqua]) {
		return [NSColor colorWithWhite:1.0 alpha:0.22];
	}
	return [NSColor colorWithWhite:0.0 alpha:0.14];
}

static CGFloat onecatchDeviceHairlineWidth(NSWindow *window) {
	CGFloat scale = window.backingScaleFactor;
	return scale > 0.0 ? 1.0 / scale : 1.0;
}

static NSColor *onecatchCanvasColor(NSAppearance *appearance) {
	NSAppearanceName match = [appearance bestMatchFromAppearancesWithNames:@[ NSAppearanceNameAqua, NSAppearanceNameDarkAqua ]];
	if ([match isEqualToString:NSAppearanceNameDarkAqua]) {
		return [NSColor colorWithSRGBRed:0x1C / 255.0 green:0x1C / 255.0 blue:0x1C / 255.0 alpha:1.0];
	}
	return [NSColor colorWithSRGBRed:0xF5 / 255.0 green:0xF5 / 255.0 blue:0xF0 / 255.0 alpha:1.0];
}

static WKWebView *onecatchFindWebView(NSView *view) {
	if ([view isKindOfClass:[WKWebView class]]) {
		return (WKWebView *)view;
	}
	for (NSView *child in view.subviews) {
		WKWebView *found = onecatchFindWebView(child);
		if (found != nil) {
			return found;
		}
	}
	return nil;
}

// The sidebar stroke sits above WebKit and opts out of hit-testing so it
// remains purely visual.
@interface OneCatchWindowBorderView : NSView
@end

@implementation OneCatchWindowBorderView
- (NSView *)hitTest:(NSPoint)point {
	return nil;
}
@end

@interface OneCatchSidebarBridge : NSObject <WKScriptMessageHandler>
@property(nonatomic, assign) NSWindow *window;
@property(nonatomic, assign) NSVisualEffectView *effectView;
@property(nonatomic, assign) NSView *canvasView;
@property(nonatomic, assign) NSView *borderView;
@end

@implementation OneCatchSidebarBridge
- (void)userContentController:(WKUserContentController *)userContentController didReceiveScriptMessage:(WKScriptMessage *)message {
	if (![message.name isEqualToString:onecatchSidebarMessageName]) {
		return;
	}

	id body = message.body;
	NSNumber *width = nil;
	NSNumber *flush = nil;
	NSNumber *hidden = nil;
	NSNumber *compact = nil;
	NSString *theme = nil;
	if ([body isKindOfClass:[NSDictionary class]]) {
		id candidateWidth = [(NSDictionary *)body objectForKey:@"width"];
		if ([candidateWidth isKindOfClass:[NSNumber class]]) {
			width = candidateWidth;
		}
		id candidateFlush = [(NSDictionary *)body objectForKey:@"flush"];
		if ([candidateFlush isKindOfClass:[NSNumber class]]) {
			flush = candidateFlush;
		}
		id candidateHidden = [(NSDictionary *)body objectForKey:@"hidden"];
		if ([candidateHidden isKindOfClass:[NSNumber class]]) {
			hidden = candidateHidden;
		}
		id candidateCompact = [(NSDictionary *)body objectForKey:@"compact"];
		if ([candidateCompact isKindOfClass:[NSNumber class]]) {
			compact = candidateCompact;
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
		BOOL useFlushRail = flush.boolValue;
		BOOL useCompactRail = !useFlushRail && compact.boolValue;
		self.effectView.frame = useFlushRail
			? NSMakeRect(NSMinX(bounds), NSMinY(bounds), MIN(width.doubleValue, NSWidth(bounds)), NSHeight(bounds))
			: useCompactRail
				? onecatchCompactSidebarPanelFrame(bounds, width.doubleValue)
				: onecatchSidebarPanelFrame(bounds, width.doubleValue);
		self.effectView.autoresizingMask = useCompactRail
			? NSViewMinYMargin | NSViewMaxXMargin
			: NSViewHeightSizable | NSViewMaxXMargin;
		self.effectView.layer.cornerRadius = useFlushRail ? 0.0 : onecatchSidebarCornerRadius;
		self.canvasView.frame = bounds;
		self.borderView.frame = self.effectView.frame;
		self.borderView.autoresizingMask = self.effectView.autoresizingMask;
		self.borderView.layer.cornerRadius = useFlushRail ? 0.0 : onecatchSidebarCornerRadius;
		self.borderView.layer.borderWidth = useFlushRail ? 0.0 : onecatchDeviceHairlineWidth(self.window);
	}
	if (hidden != nil) {
		self.effectView.hidden = hidden.boolValue;
		self.borderView.hidden = hidden.boolValue;
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
static void onecatchInstallSidebarMaterial(NSWindow *window) {
	if (objc_getAssociatedObject(window, &onecatchSidebarMaterialKey) != nil) {
		return;
	}
	WKWebView *webView = onecatchFindWebView(window.contentView);
	if (webView == nil || webView.superview == nil) {
		return;
	}

	NSView *container = webView.superview;
	NSView *canvasView = [[NSView alloc] initWithFrame:container.bounds];
	canvasView.wantsLayer = YES;
	canvasView.layer.backgroundColor = onecatchCanvasColor(window.effectiveAppearance).CGColor;
	canvasView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
	NSVisualEffectView *effectView = [[NSVisualEffectView alloc]
		initWithFrame:onecatchSidebarPanelFrame(container.bounds, 216.0)];
	effectView.material = NSVisualEffectMaterialSidebar;
	effectView.blendingMode = NSVisualEffectBlendingModeBehindWindow;
	effectView.state = NSVisualEffectStateFollowsWindowActiveState;
	effectView.emphasized = NO;
	effectView.autoresizingMask = NSViewHeightSizable | NSViewMaxXMargin;
	effectView.wantsLayer = YES;
	effectView.layer.cornerRadius = onecatchSidebarCornerRadius;
	effectView.layer.cornerCurve = kCACornerCurveContinuous;
	effectView.layer.masksToBounds = YES;
	OneCatchWindowBorderView *borderView = [[OneCatchWindowBorderView alloc] initWithFrame:effectView.frame];
	borderView.wantsLayer = YES;
	borderView.layer.backgroundColor = [NSColor clearColor].CGColor;
	borderView.layer.cornerRadius = onecatchSidebarCornerRadius;
	borderView.layer.cornerCurve = kCACornerCurveContinuous;
	borderView.layer.borderWidth = onecatchDeviceHairlineWidth(window);
	borderView.layer.borderColor = onecatchSidebarBorderColor(window.effectiveAppearance).CGColor;
	borderView.autoresizingMask = NSViewHeightSizable | NSViewMaxXMargin;
	[container addSubview:canvasView positioned:NSWindowBelow relativeTo:webView];
	[container addSubview:effectView positioned:NSWindowBelow relativeTo:webView];
	[container addSubview:borderView positioned:NSWindowAbove relativeTo:webView];

	OneCatchSidebarBridge *bridge = [[OneCatchSidebarBridge alloc] init];
	bridge.window = window;
	bridge.effectView = effectView;
	bridge.canvasView = canvasView;
	bridge.borderView = borderView;
	[webView.configuration.userContentController addScriptMessageHandler:bridge name:onecatchSidebarMessageName];

	objc_setAssociatedObject(window, &onecatchSidebarMaterialKey, effectView, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	objc_setAssociatedObject(window, &onecatchCanvasBackdropKey, canvasView, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	objc_setAssociatedObject(window, &onecatchSidebarBorderKey, borderView, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	objc_setAssociatedObject(window, &onecatchSidebarBridgeKey, bridge, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	[canvasView release];
	[effectView release];
	[borderView release];
	[bridge release];

	// Mark the native runtime only after the effect exists. Browser previews
	// retain their solid sidebar fallback. The message also catches an initial
	// React width/theme update that may have run before this handler was added.
	NSString *script =
		@"document.documentElement.dataset.nativeSidebarMaterial='true';"
		 "var sidebar=document.querySelector('.sidebar');"
		 "window.webkit.messageHandlers.onecatchSidebar.postMessage({"
		 "width:sidebar?.dataset.visible==='false'?0:(sidebar?.getBoundingClientRect().width||216),"
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
static void onecatchMatchBackgroundToCanvas(NSWindow *window) {
	NSColor *canvas = onecatchCanvasColor(window.effectiveAppearance);
	window.backgroundColor = [NSColor clearColor];
	NSView *frame = window.contentView.superview;
	frame.wantsLayer = YES;
	frame.layer.backgroundColor = [NSColor clearColor].CGColor;
	NSView *canvasView = objc_getAssociatedObject(window, &onecatchCanvasBackdropKey);
	if (canvasView.layer != nil) {
		canvasView.layer.backgroundColor = canvas.CGColor;
	}
	WKWebView *webView = onecatchFindWebView(window.contentView);
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

@interface OneCatchAppearanceObserver : NSObject
@property(nonatomic, assign) NSWindow *window;
@end

@implementation OneCatchAppearanceObserver
- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary *)change context:(void *)context {
	if ([keyPath isEqualToString:@"effectiveAppearance"]) {
		onecatchMatchBackgroundToCanvas(self.window);
		NSView *sidebarBorder = objc_getAssociatedObject(self.window, &onecatchSidebarBorderKey);
		if (sidebarBorder.layer != nil) {
			sidebarBorder.layer.borderColor = onecatchSidebarBorderColor(self.window.effectiveAppearance).CGColor;
		}
	}
}
@end

static char onecatchAppearanceObserverKey;

static void onecatchInstallAppearanceObserver(NSWindow *window) {
	if (objc_getAssociatedObject(window, &onecatchAppearanceObserverKey) != nil) {
		return;
	}
	OneCatchAppearanceObserver *observer = [[OneCatchAppearanceObserver alloc] init];
	observer.window = window;
	[window addObserver:observer forKeyPath:@"effectiveAppearance" options:NSKeyValueObservingOptionNew context:NULL];
	objc_setAssociatedObject(window, &onecatchAppearanceObserverKey, observer, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	[observer release];
}

static BOOL onecatchAcceptsFirstMouse(id self, SEL _cmd, NSEvent *event) {
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
static void onecatchEnableClickToFocus(NSWindow *window) {
	WKWebView *webView = onecatchFindWebView(window.contentView);
	if (webView == nil) {
		return;
	}
	class_addMethod([webView class], @selector(acceptsFirstMouse:), (IMP)onecatchAcceptsFirstMouse, "c@:@");
}

static void onecatchInstallNativeWindowChrome(void *handle) {
	NSWindow *window = (__bridge NSWindow *)handle;
	if (window == nil || window.contentView == nil) {
		return;
	}
	// Wails handles titlebar double-clicks from the frontend's drag regions and
	// reads the macOS preference. A content-view gesture recognizer cannot see
	// DOM no-drag controls and would also capture panel toolbars below the titlebar.
	onecatchInstallAppearanceObserver(window);
	onecatchEnableClickToFocus(window);
	onecatchMatchBackgroundToCanvas(window);
	onecatchInstallSidebarMaterial(window);
	[window invalidateShadow];
}

static void onecatchSetWindowZoomButtonHidden(void *handle, bool hidden) {
	NSWindow *window = (__bridge NSWindow *)handle;
	if (window == nil) {
		return;
	}
	NSButton *zoomButton = [window standardWindowButton:NSWindowZoomButton];
	zoomButton.enabled = !hidden;
	zoomButton.hidden = hidden;
}

static void onecatchSetApplicationIcon(void *icon, int length) {
	if (icon == nil || length <= 0) {
		return;
	}
	NSData *data = [NSData dataWithBytes:icon length:(NSUInteger)length];
	NSImage *image = [[NSImage alloc] initWithData:data];
	if (image != nil) {
		[NSApp setApplicationIconImage:image];
	}
	[image release];
}

static void onecatchSetWindowAppearance(void *targetHandle, void *sourceHandle) {
	NSWindow *target = (__bridge NSWindow *)targetHandle;
	NSWindow *source = (__bridge NSWindow *)sourceHandle;
	if (target == nil || source == nil) {
		return;
	}
	target.appearance = source.appearance;
}
*/
import "C"

import "unsafe"

func installNativeWindowChrome(window unsafe.Pointer) {
	if window == nil {
		return
	}
	C.onecatchInstallNativeWindowChrome(window)
}

func setNativeWindowZoomButtonHidden(window unsafe.Pointer, hidden bool) {
	if window == nil {
		return
	}
	C.onecatchSetWindowZoomButtonHidden(window, C.bool(hidden))
}

func setNativeApplicationIcon(icon []byte) {
	if len(icon) == 0 {
		return
	}
	C.onecatchSetApplicationIcon(unsafe.Pointer(&icon[0]), C.int(len(icon)))
}

func setNativeWindowAppearance(window, source unsafe.Pointer) {
	if window == nil || source == nil {
		return
	}
	C.onecatchSetWindowAppearance(window, source)
}
