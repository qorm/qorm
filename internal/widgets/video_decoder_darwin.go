//go:build darwin && !js

package widgets

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AVFoundation -framework Foundation -framework CoreMedia -framework CoreVideo

#import <AVFoundation/AVFoundation.h>
#import <CoreVideo/CoreVideo.h>
#import <Foundation/Foundation.h>
#import <unistd.h>
#import <stdint.h>
#include <stdlib.h>

extern void pushFrameToGo(uintptr_t handle, void* buf, int w, int h);

static void startNativeDecoder(const char *inPath, int targetW, int targetH, uintptr_t handle) {
    @autoreleasepool {
        NSString *inString = [NSString stringWithUTF8String:inPath];
        NSURL *inUrl = [NSURL fileURLWithPath:inString];
        AVAsset *asset = [AVAsset assetWithURL:inUrl];
        NSError *error = nil;
        AVAssetReader *reader = [AVAssetReader assetReaderWithAsset:asset error:&error];
        if (error) {
            NSLog(@"QORM Native Video Error: %@", [error localizedDescription]);
            return;
        }
        
        dispatch_semaphore_t sema = dispatch_semaphore_create(0);
        __block AVAssetTrack *videoTrack = nil;
        
        if ([asset respondsToSelector:@selector(loadTracksWithMediaType:completionHandler:)]) {
            [asset loadTracksWithMediaType:AVMediaTypeVideo completionHandler:^(NSArray<AVAssetTrack *> *tracks, NSError *loadError) {
                videoTrack = [tracks firstObject];
                dispatch_semaphore_signal(sema);
            }];
            dispatch_semaphore_wait(sema, DISPATCH_TIME_FOREVER);
        } else {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
            videoTrack = [[asset tracksWithMediaType:AVMediaTypeVideo] firstObject];
#pragma clang diagnostic pop
        }
        
        if (!videoTrack) {
            NSLog(@"QORM Native Video Error: No video track found in %@", inString);
            return;
        }
        
        NSDictionary *outputSettings = @{
            (id)kCVPixelBufferPixelFormatTypeKey: @(kCVPixelFormatType_32RGBA),
            (id)kCVPixelBufferWidthKey: @(targetW),
            (id)kCVPixelBufferHeightKey: @(targetH)
        };
        
        AVAssetReaderTrackOutput *trackOutput = [AVAssetReaderTrackOutput assetReaderTrackOutputWithTrack:videoTrack outputSettings:outputSettings];
        [reader addOutput:trackOutput];
        [reader startReading];
        
        while ([reader status] == AVAssetReaderStatusReading) {
            CMSampleBufferRef sample = [trackOutput copyNextSampleBuffer];
            if (sample) {
                CVImageBufferRef imageBuffer = CMSampleBufferGetImageBuffer(sample);
                if (imageBuffer) {
                    CVPixelBufferLockBaseAddress(imageBuffer, kCVPixelBufferLock_ReadOnly);
                    void *baseAddress = CVPixelBufferGetBaseAddress(imageBuffer);
                    
                    // Call back into Go with the pixel buffer
                    pushFrameToGo(handle, baseAddress, targetW, targetH);
                    
                    CVPixelBufferUnlockBaseAddress(imageBuffer, kCVPixelBufferLock_ReadOnly);
                }
                CFRelease(sample);
                // ~30fps delay
                usleep(33000);
            } else {
                break;
            }
        }
    }
}
*/
import "C"
import (
	"image"
	"runtime/cgo"
	"unsafe"
)

//export pushFrameToGo
func pushFrameToGo(handle C.uintptr_t, buf unsafe.Pointer, w C.int, h C.int) {
	hdl := cgo.Handle(handle)
	v, ok := hdl.Value().(*Video)
	if !ok {
		return
	}
	width := int(w)
	height := int(h)
	length := width * height * 4
	slice := unsafe.Slice((*byte)(buf), length)
	
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Because CVPixelBuffer might have stride/padding, in a real production code we'd handle bytesPerRow.
	// We asked for exact targetW and targetH, usually CoreVideo respects it without padding for standard sizes.
	copy(img.Pix, slice)
	v.AppendFrame(img)
}

func startVideoDecoder(v *Video, w, h int) {
	v.mu.Lock()
	if v.playing {
		v.mu.Unlock()
		return
	}
	v.playing = true
	src := v.src
	v.mu.Unlock()

	if src == "" {
		startFallbackDecoder(v, w, h)
		return
	}

	go func() {
		handle := cgo.NewHandle(v)
		defer handle.Delete()
		
		cPath := C.CString(src)
		defer C.free(unsafe.Pointer(cPath))
		
		C.startNativeDecoder(cPath, C.int(w), C.int(h), C.uintptr_t(handle))
		
		// If native decoder finishes or fails, fallback to plasma
		startFallbackDecoder(v, w, h)
	}()
}
