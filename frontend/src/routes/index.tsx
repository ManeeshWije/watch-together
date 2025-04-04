import { createFileRoute, Navigate } from "@tanstack/react-router";
import Nav from "../components/Nav";
import { useAuthQuery } from "../utils";
import Spinner from "../components/Spinner";

export const Route = createFileRoute("/")({
    component: Index,
});

function Index() {
    const { data: authData, isLoading: authLoading, error: authError } = useAuthQuery();

    if (authLoading) return <Spinner />;

    return <>{authError || !authData?.authenticated ? <Nav /> : <Navigate to="/video" />}</>;
}
