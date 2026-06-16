package com.limyedb.models;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.util.List;
import java.util.Map;

/**
 * Represents a vector point in a collection.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public class Point {

    @JsonProperty("id")
    private String id;

    @JsonProperty("vector")
    private List<Float> vector;

    @JsonProperty("payload")
    private Map<String, Object> payload;

    @JsonProperty("named_vectors")
    private Map<String, List<Float>> namedVectors;

    @JsonProperty("sparse")
    private SparseVector sparse;

    public Point() {
    }

    public Point(String id, List<Float> vector) {
        this.id = id;
        this.vector = vector;
    }

    public Point(String id, List<Float> vector, Map<String, Object> payload) {
        this.id = id;
        this.vector = vector;
        this.payload = payload;
    }

    public String getId() {
        return id;
    }

    public Point setId(String id) {
        this.id = id;
        return this;
    }

    public List<Float> getVector() {
        return vector;
    }

    public Point setVector(List<Float> vector) {
        this.vector = vector;
        return this;
    }

    public Map<String, Object> getPayload() {
        return payload;
    }

    public Point setPayload(Map<String, Object> payload) {
        this.payload = payload;
        return this;
    }

    public Map<String, List<Float>> getNamedVectors() {
        return namedVectors;
    }

    public Point setNamedVectors(Map<String, List<Float>> namedVectors) {
        this.namedVectors = namedVectors;
        return this;
    }

    public SparseVector getSparse() {
        return sparse;
    }

    public Point setSparse(SparseVector sparse) {
        this.sparse = sparse;
        return this;
    }

    /**
     * Creates a builder for Point.
     */
    public static Builder builder() {
        return new Builder();
    }

    public static class Builder {
        private String id;
        private List<Float> vector;
        private Map<String, Object> payload;
        private Map<String, List<Float>> namedVectors;
        private SparseVector sparse;

        public Builder id(String id) {
            this.id = id;
            return this;
        }

        public Builder vector(List<Float> vector) {
            this.vector = vector;
            return this;
        }

        public Builder vector(float... values) {
            this.vector = new java.util.ArrayList<>();
            for (float v : values) {
                this.vector.add(v);
            }
            return this;
        }

        public Builder payload(Map<String, Object> payload) {
            this.payload = payload;
            return this;
        }

        public Builder namedVectors(Map<String, List<Float>> namedVectors) {
            this.namedVectors = namedVectors;
            return this;
        }

        public Builder sparse(SparseVector sparse) {
            this.sparse = sparse;
            return this;
        }

        public Point build() {
            Point point = new Point();
            point.id = this.id;
            point.vector = this.vector;
            point.payload = this.payload;
            point.namedVectors = this.namedVectors;
            point.sparse = this.sparse;
            return point;
        }
    }
}
