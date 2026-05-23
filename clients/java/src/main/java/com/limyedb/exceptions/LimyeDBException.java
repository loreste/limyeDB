package com.limyedb.exceptions;

/**
 * Base exception for all LimyeDB client errors.
 */
public class LimyeDBException extends Exception {
    private final int statusCode;

    public LimyeDBException(String message) {
        super(message);
        this.statusCode = 0;
    }

    public LimyeDBException(String message, int statusCode) {
        super(message);
        this.statusCode = statusCode;
    }

    public LimyeDBException(String message, Throwable cause) {
        super(message, cause);
        this.statusCode = 0;
    }

    public LimyeDBException(String message, int statusCode, Throwable cause) {
        super(message, cause);
        this.statusCode = statusCode;
    }

    /**
     * Returns the HTTP status code associated with this error, or 0 if not applicable.
     */
    public int getStatusCode() {
        return statusCode;
    }
}
