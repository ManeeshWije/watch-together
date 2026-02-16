import { useQuery } from "@tanstack/react-query";
import { AuthResponse, User, VideoData } from "./types";

const API_URL =
  import.meta.env.MODE === "production"
    ? ""
    : "http://localhost:8080";

async function apiFetch<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const response = await fetch(`${API_URL}${path}`, {
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...options.headers,
    },
    ...options,
  });

  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || "Request failed");
  }

  const data: unknown = await response.json();

  return data as T;
}

// Auth
export const fetchAuthSession = () =>
  apiFetch<AuthResponse>("/auth/session");

// Videos
export const fetchVideos = () =>
  apiFetch<VideoData[]>("/list-videos");

export const getVideo = (videoTitle: string) =>
  apiFetch<VideoData>(
    `/get-video/${encodeURIComponent(videoTitle)}`
  );

export const addVideo = (videoUrl: string) =>
  apiFetch<VideoData>("/add-video", {
    method: "POST",
    body: JSON.stringify({ url: videoUrl }),
  });

export const deleteVideo = (videoTitle: string) =>
  apiFetch<void>(
    `/delete-video/${encodeURIComponent(videoTitle)}`,
    {
      method: "POST",
    }
  );

// Users
export const fetchConnectedUsers = () =>
  apiFetch<User[]>("/users");

export const useAuthQuery = () => {
  return useQuery({
    queryKey: ["authSession"],
    queryFn: fetchAuthSession,
    staleTime: 1000 * 60 * 5, // 5 minutes
    retry: false,
  });
};
