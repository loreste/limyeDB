namespace LimyeDB.Exceptions;

/// <summary>
/// Thrown when the requested collection does not exist.
/// </summary>
public class CollectionNotFoundException : LimyeDBException
{
    /// <summary>
    /// The name of the collection that was not found.
    /// </summary>
    public string CollectionName { get; }

    public CollectionNotFoundException(string collectionName)
        : base($"Collection not found: {collectionName}", 404)
    {
        CollectionName = collectionName;
    }

    public CollectionNotFoundException(string collectionName, string message)
        : base(message, 404)
    {
        CollectionName = collectionName;
    }
}
