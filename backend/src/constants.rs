use std::time::Duration;

pub const COOKIE_AUTH_SESSION: &str = "auth_session";
pub const COOKIE_AUTH_CSRF_STATE: &str = "auth_csrf_state";
pub const COOKIE_AUTH_CODE_VERIFIER: &str = "auth_code_verifier";
pub const SESSION_DURATION: Duration = Duration::from_millis(1000 * 60 * 60 * 24); // 24 hours
//pub const SESSION_DURATION: Duration = Duration::from_millis(1000 * 60 * 1); // 1 minute
