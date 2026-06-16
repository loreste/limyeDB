package com.limyedb.exceptions;

/**
 * Thrown when the client fails to connect to the LimyeDB server.
 */
public class ConnectionException extends LimyeDBException {

    public ConnectionException(String message) {
        super(message);
    }

    public ConnectionException(String message, Throwable cause) {
        super(message, cause);
    }
}
