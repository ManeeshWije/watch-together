import { useAuthQuery } from "../utils";
import Spinner from "./Spinner";

export default function Nav() {
    const { data: authData, isLoading: authLoading, error: authError } = useAuthQuery();
    if (authLoading) return <Spinner />;
    const serverUrl = import.meta.env.MODE === "production" ? "" : "http://localhost:8080";

    return (
        <div className="text-white flex flex-col text-center justify-center items-center p-4 gap-2">
            <a className="hover:opacity-85" href="https://www.github.com/ManeeshWije/watch-together">
                Source Code
            </a>
            <h1 className="font-bold text-3xl">Watch Together</h1>
            <p>A simple way to watch videos in real-time with multiple connected clients</p>
            {!authError && authData?.user ? (
                <a href={`${serverUrl}/auth/logout`}>
                    <button className="bg-red-500 p-2 rounded-md hover:opacity-85">Logout</button>
                </a>
            ) : (
                <a href={`${serverUrl}/auth/google/login`}>
                    <button className="bg-blue-500 p-2 rounded-md hover:opacity-85">Login with Google</button>
                </a>
            )}
        </div>
    );
}
