package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AVFoundation -framework Foundation -framework CoreMedia -framework CoreVideo

#import <AVFoundation/AVFoundation.h>
#import <CoreVideo/CoreVideo.h>
#import <Foundation/Foundation.h>
#import <unistd.h>

void streamVideo(const char *inPath, int targetW, int targetH) {
    @autoreleasepool {
        NSString *inString = [NSString stringWithUTF8String:inPath];
        NSURL *inUrl = [NSURL fileURLWithPath:inString];
        AVAsset *asset = [AVAsset assetWithURL:inUrl];
        NSError *error = nil;
        AVAssetReader *reader = [AVAssetReader assetReaderWithAsset:asset error:&error];
        if (error) return;
        
        AVAssetTrack *videoTrack = [[asset tracksWithMediaType:AVMediaTypeVideo] firstObject];
        if (!videoTrack) return;
        
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
                    size_t bytesPerRow = CVPixelBufferGetBytesPerRow(imageBuffer);
                    size_t height = CVPixelBufferGetHeight(imageBuffer);
                    
                    // We only write exactly targetW * 4 bytes per row to avoid padding issues
                    size_t targetRowBytes = targetW * 4;
                    for (size_t y = 0; y < height && y < targetH; y++) {
                        void *rowStart = baseAddress + (y * bytesPerRow);
                        fwrite(rowStart, 1, targetRowBytes, stdout);
                    }
                    fflush(stdout);
                    
                    CVPixelBufferUnlockBaseAddress(imageBuffer, kCVPixelBufferLock_ReadOnly);
                }
                CFRelease(sample);
                // ~30fps delay so we don't flood the pipe instantly
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
	"os"
	"strconv"
)

func main() {
	if len(os.Args) < 4 {
		os.Exit(1)
	}
	path := C.CString(os.Args[1])
	w, _ := strconv.Atoi(os.Args[2])
	h, _ := strconv.Atoi(os.Args[3])
	C.streamVideo(path, C.int(w), C.int(h))
}
