package com.limyedb.exceptions;

/**
 * Thrown when authentication with the LimyeDB server fails.
 */
public class AuthenticationException extends LimyeDBException {

    public AuthenticationException(String message) {
        super(message, 401);
    }

    public AuthenticationException(String message, Throwable cause) {
        super(message, 401, cause);
    }
}
