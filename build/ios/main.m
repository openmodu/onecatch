//go:build ios
#import <UIKit/UIKit.h>
#include <stdio.h>

// Wails provides this class in the Go archive. Keep the bridge here so the
// application does not need to modify a downloaded framework's source.
@interface WailsAppDelegate : UIResponder <UIApplicationDelegate>
@property (strong, nonatomic) UIWindow *window;
@end

@interface OneCatchSceneDelegate : UIResponder <UIWindowSceneDelegate>
@property (strong, nonatomic) UIWindow *window;
@end

@interface OneCatchAppDelegate : WailsAppDelegate
- (void)connectWindowScene:(UIWindowScene *)scene;
@end

@implementation OneCatchAppDelegate
- (BOOL)application:(UIApplication *)application didFinishLaunchingWithOptions:(NSDictionary *)options {
    // Wails starts Go and creates its WebView in this callback. Defer that
    // work until UIKit supplies the scene and its window is ready.
    return YES;
}

- (UISceneConfiguration *)application:(UIApplication *)application
    configurationForConnectingSceneSession:(UISceneSession *)session
    options:(UISceneConnectionOptions *)options {
    UISceneConfiguration *configuration = [[UISceneConfiguration alloc] initWithName:@"OneCatch" sessionRole:session.role];
    configuration.delegateClass = [OneCatchSceneDelegate class];
    return configuration;
}

- (void)connectWindowScene:(UIWindowScene *)scene {
    static dispatch_once_t onceToken;
    dispatch_once(&onceToken, ^{
        UIColor *background = [UIColor colorNamed:@"LaunchBackground"] ?: [UIColor systemBackgroundColor];
        self.window = [[UIWindow alloc] initWithWindowScene:scene];
        UIViewController *root = [[UIViewController alloc] init];
        root.view.backgroundColor = background;
        self.window.rootViewController = root;
        self.window.backgroundColor = background;
        // The framework retains this same window and replaces its controller
        // with the WebView. Start its runtime once, after scene connection.
        [super application:UIApplication.sharedApplication didFinishLaunchingWithOptions:nil];
    });
    self.window.windowScene = scene;
    [self.window makeKeyAndVisible];
}
@end

@implementation OneCatchSceneDelegate
- (void)scene:(UIScene *)scene willConnectToSession:(UISceneSession *)session options:(UISceneConnectionOptions *)options {
    if (![scene isKindOfClass:[UIWindowScene class]]) return;
    OneCatchAppDelegate *delegate = (OneCatchAppDelegate *)UIApplication.sharedApplication.delegate;
    [delegate connectWindowScene:(UIWindowScene *)scene];
    self.window = delegate.window;
}

// Wails' application events remain available to Go under the scene lifecycle.
- (void)sceneDidBecomeActive:(UIScene *)scene {
    [(OneCatchAppDelegate *)UIApplication.sharedApplication.delegate applicationDidBecomeActive:UIApplication.sharedApplication];
}
- (void)sceneWillResignActive:(UIScene *)scene {
    [(OneCatchAppDelegate *)UIApplication.sharedApplication.delegate applicationWillResignActive:UIApplication.sharedApplication];
}
- (void)sceneWillEnterForeground:(UIScene *)scene {
    [(OneCatchAppDelegate *)UIApplication.sharedApplication.delegate applicationWillEnterForeground:UIApplication.sharedApplication];
}
- (void)sceneDidEnterBackground:(UIScene *)scene {
    [(OneCatchAppDelegate *)UIApplication.sharedApplication.delegate applicationDidEnterBackground:UIApplication.sharedApplication];
}
@end

int main(int argc, char *argv[]) {
    @autoreleasepool {
        setvbuf(stdout, NULL, _IONBF, 0);
        setvbuf(stderr, NULL, _IONBF, 0);
        return UIApplicationMain(argc, argv, nil, @"OneCatchAppDelegate");
    }
}
