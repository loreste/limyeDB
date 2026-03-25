namespace LimyeDB.Exceptions;

/// <summary>
/// Base exception for all LimyeDB client errors.
/// </summary>
public class LimyeDBException : Exception
{
    /// <summary>
    /// The HTTP status code associated with this error, or 0 if not applicable.
    /// </summary>
    public int StatusCode { get; }

    public LimyeDBException(string message) : base(message)
    {
        StatusCode = 0;
    }

    public LimyeDBException(string message, int statusCode) : base(message)
    {
        StatusCode = statusCode;
    }

    public LimyeDBException(string message, Exception innerException) : base(message, innerException)
    {
        StatusCode = 0;
    }

    public LimyeDBException(string message, int statusCode, Exception innerException) : base(message, innerException)
    {
        StatusCode = statusCode;
    }
}
