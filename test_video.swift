import AVFoundation
import CoreImage

let url = URL(fileURLWithPath: "examples/canvas-ultimate/assets/video.mp4")
let asset = AVAsset(url: url)
print("Asset loaded, duration: \(asset.duration.seconds)")
