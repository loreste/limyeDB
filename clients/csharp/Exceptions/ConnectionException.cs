namespace LimyeDB.Exceptions;

/// <summary>
/// Thrown when the client fails to connect to the LimyeDB server.
/// </summary>
public class ConnectionException : LimyeDBException
{
    public ConnectionException(string message) : base(message)
    {
    }

    public ConnectionException(string message, Exception innerException) : base(message, innerException)
    {
    }
}
