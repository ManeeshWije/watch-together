export interface User {
    uuid: string;
    username: string;
    email: string;
    created_at: string;
    num_uploads: number;
    is_admin: boolean;
}

export interface UserSession {
    uuid: string;
    user_uuid: string;
    created_at: string;
    expires_at: string;
}

export interface AuthResponse {
    authenticated: boolean;
    user: User;
}

export interface VideoData {
    title: string;
    url: string;
    size: number;
}
