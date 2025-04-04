import { useEffect, useRef, useState } from "react";
import { createFileRoute, Navigate } from "@tanstack/react-router";
import Nav from "../components/Nav";
import { VideoData } from "../types";
import { useQuery } from "@tanstack/react-query";
import { useAuthQuery } from "../utils";
import Spinner from "../components/Spinner";

const serverUrl = import.meta.env.MODE === "production" ? "" : "http://localhost:8080";

async function fetchVideos(): Promise<VideoData[]> {
    const response = await fetch(`${serverUrl}/list-videos`, {
        credentials: "include",
    });
    if (!response.ok) {
        throw new Error("Failed to fetch videos");
    }
    return response.json();
}

export const Route = createFileRoute("/video")({
    component: Video,
});

function Video() {
    const videoRef = useRef<HTMLVideoElement | null>(null);
    const [socket, setSocket] = useState<WebSocket | null>(null);
    const [isSyncing, setIsSyncing] = useState(false);
    const [progress, setProgress] = useState(0);
    const [videoUrl, setVideoUrl] = useState("");
    const [loading, setLoading] = useState(false);
    const [errorMessage, setErrorMessage] = useState("");
    const [_videoChunks, setVideoChunks] = useState<Uint8Array[]>([]);
    const [_videoBlob, setVideoBlob] = useState<Blob | null>(null);
    const [receivedSize, setReceivedSize] = useState(0);
    const [totalSize, setTotalSize] = useState(0);

    const { data: authData, isLoading: authLoading, error: authError } = useAuthQuery();

    const {
        data: videos,
        error,
        isLoading,
        refetch,
    } = useQuery<VideoData[], Error>({
        queryKey: ["videos"],
        queryFn: fetchVideos,
    });

    useEffect(() => {
        if (!socket) {
            const newSocket = new WebSocket(`${serverUrl}/ws`);
            newSocket.binaryType = "arraybuffer";
            setSocket(newSocket);

            newSocket.onopen = () => console.log("Connected to WebSocket");
            newSocket.onclose = () => console.log("Disconnected from WebSocket");
            newSocket.onerror = (e) => console.error("WebSocket error:", e);

            newSocket.onmessage = (event) => {
                console.log("WebSocket message received:", event);
                if (typeof event.data === "string") {
                    handleStringMessage(event.data);
                } else if (event.data instanceof ArrayBuffer) {
                    handleBinaryMessage(event.data);
                } else {
                    console.error("Unknown WebSocket message type");
                }
            };
        }
    }, [socket]);

    function handleStringMessage(message: string) {
        const [command, value] = message.split(":");
        switch (command) {
            case "TIMESTAMP":
                syncVideoTime(parseFloat(value));
                break;
            case "PLAY":
                videoRef.current?.play();
                break;
            case "PAUSE":
                videoRef.current?.pause();
                break;
            case "PROGRESS":
                setProgress(parseFloat(value));
                break;
            default:
                console.warn("Unknown command received:", command);
        }
    }

    function handleBinaryMessage(data: ArrayBuffer) {
        // Convert the incoming chunk (ArrayBuffer) to Uint8Array and append it
        const newChunk = new Uint8Array(data);
        setVideoChunks((prevChunks) => {
            const updatedChunks = [...prevChunks, newChunk];
            const combinedBlob = new Blob(updatedChunks, { type: "video/webm" });
            setVideoBlob(combinedBlob); // Update the video Blob
            setReceivedSize((prevSize) => prevSize + newChunk.length);

            // Update the video URL only when a new Blob is ready
            if (videoRef.current) {
                videoRef.current.src = URL.createObjectURL(combinedBlob);
            }

            return updatedChunks;
        });
    }

    function syncVideoTime(timestamp: number) {
        if (!isSyncing && videoRef.current) {
            setIsSyncing(true);
            videoRef.current.currentTime = timestamp / 1000;
            setTimeout(() => setIsSyncing(false), 500);
        }
    }

    async function handleVideoClick(videoTitle: string) {
        setLoading(true);
        setVideoChunks([]);
        setVideoBlob(null);
        setReceivedSize(0);
        setTotalSize(0);
        setProgress(0);
        const selectedVideo = videos?.find((video) => video.title === videoTitle);
        if (selectedVideo) {
            setTotalSize(selectedVideo.size);
        }

        try {
            const response = await fetch(`${serverUrl}/get-video/${videoTitle}`, {
                credentials: "include",
            });
            if (!response.ok) {
                throw new Error("Failed to request video stream");
            }
            console.log("Requested video stream from backend");
        } catch (error) {
            console.error("Error fetching video:", error);
        } finally {
            setLoading(false);
        }
    }

    async function handleSubmit(e: React.FormEvent) {
        e.preventDefault();
        try {
            setVideoChunks([]);
            setVideoBlob(null);
            setReceivedSize(0);
            setTotalSize(0);
            setProgress(0);
            setLoading(true);
            setErrorMessage("");
            const response = await fetch(`${serverUrl}/add-video`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ url: videoUrl }),
                credentials: "include",
            });

            if (!response.ok) throw new Error("Failed to add video.");

            await response.json();
            setVideoUrl("");
            refetch();
        } catch (error) {
            setErrorMessage("Error adding video. Please try again.");
        } finally {
            setLoading(false);
        }
    }

    async function handleVideoDelete(videoTitle: string) {
        try {
            setLoading(true);
            setVideoChunks([]);
            setVideoBlob(null);
            setReceivedSize(0);
            setTotalSize(0);
            setProgress(0);
            const response = await fetch(`${serverUrl}/delete-video/${videoTitle}`, {
                credentials: "include",
                method: "POST",
            });
            if (!response.ok) {
                throw new Error("Failed to delete video stream");
            }
            console.log("Requested video stream deletion from backend");

            // Remove the deleted video from the list immediately
            if (videos) {
                refetch();
            }
        } catch (error) {
            console.error("Error deleting video:", error);
        } finally {
            setLoading(false);
        }
    }

    // Adjust the progress bar to show chunks after 100% is reached
    const calculateProgress = () => {
        if (totalSize === 0) return 0; // Avoid division by zero

        if (receivedSize < totalSize) {
            return (receivedSize / totalSize) * 100;
        }

        // Once the received size is equal or greater than the total size, display progress as 100%
        return 100;
    };

    if (!authLoading && (authError || !authData?.authenticated)) {
        return <Navigate to="/" />;
    }
    if (authLoading) return <Spinner />;

    return (
        <>
            <Nav />
            {/* Input Box */}
            <form onSubmit={handleSubmit} className="mt-4 max-w-lg mx-auto flex gap-2 text-white">
                <input type="text" value={videoUrl} onChange={(e) => setVideoUrl(e.target.value)} placeholder="Enter YouTube Video URL" className="border p-2 w-full rounded" />
                <button type="submit" className="bg-blue-500 text-white px-4 py-2 rounded hover:bg-blue-600 transition" disabled={loading}>
                    Submit
                </button>
            </form>

            {/* Error Message */}
            {errorMessage && <div className="error-message mt-4 text-red-500 text-center">{errorMessage}</div>}

            {/* List of Videos */}
            <div className="video-list mt-6 w-full max-w-lg mx-auto">
                {isLoading && <p className="text-gray-400 text-center">Loading videos...</p>}
                {error && <p className="text-red-500 text-center">Error loading videos: {error.message}</p>}
                {videos && videos.length === 0 && <p className="text-gray-500 text-center">No videos available.</p>}
                <ul className="space-y-3">
                    {videos?.map((video) => (
                        <li key={video.url} className="flex justify-between items-center bg-gray-800 p-3 rounded-lg shadow">
                            <button onClick={() => handleVideoClick(video.title)} className="text-blue-400 hover:text-blue-300 font-medium">
                                {video.title}
                            </button>
                            <button onClick={() => handleVideoDelete(video.title)} className="bg-red-500 text-white px-3 py-1 rounded-md hover:bg-red-600 transition">
                                Delete
                            </button>
                        </li>
                    ))}
                </ul>
            </div>

            {/* Video Player */}
            <div className="p-2 flex flex-col justify-center items-center">
                <video
                    className="w-full h-auto max-w-[900px] max-h-[600px]"
                    ref={videoRef}
                    controls
                    onPlay={() => socket?.send("PLAY")}
                    onPause={() => socket?.send("PAUSE")}
                    onSeeked={() => {
                        if (!isSyncing && videoRef.current) {
                            socket?.send(`TIMESTAMP:${videoRef.current.currentTime * 1000}`);
                        }
                    }}
                />
                {receivedSize < totalSize && !loading && (
                    <div className="w-full mt-4">
                        <div className="bg-gray-300 w-full h-2 rounded-full">
                            <div className="bg-blue-500 h-2 rounded-full" style={{ width: `${calculateProgress()}%` }}></div>
                        </div>
                        <p className="text-center text-gray-400 mt-2">{`Received: ${Math.round(calculateProgress())}%`}</p>
                    </div>
                )}
                {/* Progress Bar */}
                {progress > 0 && progress < 100 && (
                    <div className="w-full mt-4">
                        <div className="bg-gray-300 w-full h-2 rounded-full">
                            <div className="bg-blue-500 h-2 rounded-full" style={{ width: `${progress}%` }}></div>
                        </div>
                        <p className="text-center text-gray-400 mt-2">{`Uploading: ${Math.round(progress)}%`}</p>
                    </div>
                )}
            </div>

            {/* Display the Spinner only if the loading state is true */}
            {loading && progress === 0 && receivedSize === 0 && (
                <div className="spinner-container absolute top-1/2 left-1/2 transform -translate-x-1/2 -translate-y-1/2 z-50">
                    <Spinner />
                </div>
            )}
        </>
    );
}
