namespace LimyeDB.Exceptions;

/// <summary>
/// Thrown when authentication with the LimyeDB server fails.
/// </summary>
public class AuthenticationException : LimyeDBException
{
    public AuthenticationException(string message) : base(message, 401)
    {
    }

    public AuthenticationException(string message, Exception innerException) : base(message, 401, innerException)
    {
    }
}
