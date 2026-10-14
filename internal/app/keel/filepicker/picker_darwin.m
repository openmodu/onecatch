//go:build darwin && cgo

#import <AppKit/AppKit.h>
#include <stdint.h>
extern void onecatch_files_chosen(uintptr_t token, const char *json, int failed);

void onecatch_choose_files(uintptr_t token, const char *title, int directory, int multiple) {
    NSString *caption = title ? [NSString stringWithUTF8String:title] : @"";
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            NSWindow *window = NSApp.keyWindow ?: NSApp.mainWindow;
            if (!window) { onecatch_files_chosen(token, "[]", 1); return; }
            NSOpenPanel *panel = [NSOpenPanel openPanel];
            panel.title = caption;
            panel.canChooseDirectories = directory != 0;
            panel.canChooseFiles = directory == 0;
            panel.allowsMultipleSelection = multiple != 0;
            panel.canCreateDirectories = directory != 0;
            [panel beginSheetModalForWindow:window completionHandler:^(NSModalResponse response) {
                NSMutableArray *paths = [NSMutableArray array];
                if (response == NSModalResponseOK) {
                    for (NSURL *url in panel.URLs) {
                        if (url.isFileURL && url.path) [paths addObject:url.path];
                    }
                }
                NSData *data = [NSJSONSerialization dataWithJSONObject:paths options:0 error:nil];
                NSString *json = data ? [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] : nil;
                onecatch_files_chosen(token, json ? json.UTF8String : "[]", json ? 0 : 1);
            }];
        }
    });
}
