import { useQuery } from "@tanstack/react-query";
import { AuthResponse, User, VideoData } from "./types";

const serverUrl = import.meta.env.MODE === "production" ? "" : "http://localhost:8080";

const fetchAuthSession = async (): Promise<AuthResponse> => {
    const response = await fetch(`${serverUrl}/auth/session`, {
        credentials: "include",
    });
    if (!response.ok) {
        throw new Error("Failed to fetch auth session");
    }
    console.log("Fetched auth session successfully");
    return response.json();
};

export const fetchVideos = async (): Promise<VideoData[]> => {
    const response = await fetch(`${serverUrl}/list-videos`, {
        credentials: "include",
    });
    if (!response.ok) {
        throw new Error("Failed to fetch videos");
    }
    console.log("Fetched all videos successfully");
    return response.json();
};

export const fetchConnectedUsers = async (): Promise<User[]> => {
    const response = await fetch(`${serverUrl}/users`, {
        credentials: "include",
    });
    if (!response.ok) {
        throw new Error("Failed to fetch all connected users");
    }
    console.log("Connected users successfully fetched");
    return response.json();
};

export const getVideo = async (videoTitle: string): Promise<VideoData> => {
    const response = await fetch(`${serverUrl}/get-video/${videoTitle}`, {
        credentials: "include",
    });
    if (!response.ok) {
        throw new Error("Failed to request video stream");
    }
    console.log("Video successfully fetched");
    return response.json();
};

export const addVideo = async (videoUrl: string) => {
    const response = await fetch(`${serverUrl}/add-video`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ url: videoUrl }),
        credentials: "include",
    });
    if (!response.ok) {
        throw new Error("Failed to add video.");
    }
    console.log("Video successfully added");
    return response.json();
};

export const deleteVideo = async (videoTitle: string) => {
    const response = await fetch(`${serverUrl}/delete-video/${videoTitle}`, {
        credentials: "include",
        method: "POST",
    });
    if (!response.ok) {
        throw new Error("Failed to delete video stream");
    }
    console.log("Requested video stream deletion from backend");
    return response.json();
};

export const useAuthQuery = () => {
    return useQuery({
        queryKey: ["authSession"],
        queryFn: fetchAuthSession,
        staleTime: 1000 * 60 * 5, // Cache for 5 minutes
        retry: false, // Don't retry failed requests
    });
};

