package com.limyedb.exceptions;

/**
 * Thrown when the requested collection does not exist.
 */
public class CollectionNotFoundException extends LimyeDBException {
    private final String collectionName;

    public CollectionNotFoundException(String collectionName) {
        super("Collection not found: " + collectionName, 404);
        this.collectionName = collectionName;
    }

    public CollectionNotFoundException(String collectionName, String message) {
        super(message, 404);
        this.collectionName = collectionName;
    }

    /**
     * Returns the name of the collection that was not found.
     */
    public String getCollectionName() {
        return collectionName;
    }
}
