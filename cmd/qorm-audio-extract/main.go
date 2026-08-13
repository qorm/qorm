package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AVFoundation -framework Foundation -framework CoreMedia

#import <AVFoundation/AVFoundation.h>
#import <Foundation/Foundation.h>

int extractAudio(const char *inPath, const char *outPath) {
    @autoreleasepool {
        NSString *inString = [NSString stringWithUTF8String:inPath];
        NSString *outString = [NSString stringWithUTF8String:outPath];
        NSURL *inUrl = [NSURL fileURLWithPath:inString];
        NSURL *outUrl = [NSURL fileURLWithPath:outString];
        
        AVAsset *asset = [AVAsset assetWithURL:inUrl];
        NSError *error = nil;
        AVAssetReader *reader = [AVAssetReader assetReaderWithAsset:asset error:&error];
        if (error) return 1;
        
        AVAssetTrack *audioTrack = [[asset tracksWithMediaType:AVMediaTypeAudio] firstObject];
        if (!audioTrack) return 2;
        
        NSDictionary *outputSettings = @{
            AVFormatIDKey: @(kAudioFormatLinearPCM),
            AVSampleRateKey: @44100,
            AVNumberOfChannelsKey: @2,
            AVLinearPCMBitDepthKey: @16,
            AVLinearPCMIsFloatKey: @NO,
            AVLinearPCMIsBigEndianKey: @NO,
            AVLinearPCMIsNonInterleavedKey: @NO
        };
        
        AVAssetReaderTrackOutput *trackOutput = [AVAssetReaderTrackOutput assetReaderTrackOutputWithTrack:audioTrack outputSettings:outputSettings];
        [reader addOutput:trackOutput];
        [reader startReading];
        
        AVAssetWriter *writer = [AVAssetWriter assetWriterWithURL:outUrl fileType:AVFileTypeWAVE error:&error];
        if (error) return 3;
        
        AVAssetWriterInput *writerInput = [AVAssetWriterInput assetWriterInputWithMediaType:AVMediaTypeAudio outputSettings:outputSettings];
        writerInput.expectsMediaDataInRealTime = NO;
        [writer addInput:writerInput];
        [writer startWriting];
        [writer startSessionAtSourceTime:kCMTimeZero];
        
        dispatch_queue_t queue = dispatch_queue_create("audioQueue", NULL);
        dispatch_group_t group = dispatch_group_create();
        dispatch_group_enter(group);
        
        [writerInput requestMediaDataWhenReadyOnQueue:queue usingBlock:^{
            while ([writerInput isReadyForMoreMediaData]) {
                CMSampleBufferRef sample = [trackOutput copyNextSampleBuffer];
                if (sample) {
                    [writerInput appendSampleBuffer:sample];
                    CFRelease(sample);
                } else {
                    [writerInput markAsFinished];
                    dispatch_group_leave(group);
                    break;
                }
            }
        }];
        
        dispatch_group_wait(group, DISPATCH_TIME_FOREVER);
        [writer finishWritingWithCompletionHandler:^{}];
        
        return 0;
    }
}
*/
import "C"
import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: extract <video.mp4> <output.wav>")
		os.Exit(1)
	}
	inPath := C.CString(os.Args[1])
	outPath := C.CString(os.Args[2])
	
	// Remove output file if it exists
	os.Remove(os.Args[2])
	
	res := C.extractAudio(inPath, outPath)
	if res != 0 {
		fmt.Printf("Extraction failed with code %d\n", int(res))
		os.Exit(int(res))
	}
	fmt.Println("Audio successfully extracted via Go AVFoundation bindings!")
}
