package com.limyedb.models;

import com.fasterxml.jackson.annotation.JsonProperty;

import java.util.List;

/**
 * Represents a sparse vector with indices and values.
 */
public class SparseVector {

    @JsonProperty("indices")
    private List<Integer> indices;

    @JsonProperty("values")
    private List<Float> values;

    public SparseVector() {
    }

    public SparseVector(List<Integer> indices, List<Float> values) {
        this.indices = indices;
        this.values = values;
    }

    public List<Integer> getIndices() {
        return indices;
    }

    public SparseVector setIndices(List<Integer> indices) {
        this.indices = indices;
        return this;
    }

    public List<Float> getValues() {
        return values;
    }

    public SparseVector setValues(List<Float> values) {
        this.values = values;
        return this;
    }

    /**
     * Creates a builder for SparseVector.
     */
    public static Builder builder() {
        return new Builder();
    }

    public static class Builder {
        private List<Integer> indices;
        private List<Float> values;

        public Builder indices(List<Integer> indices) {
            this.indices = indices;
            return this;
        }

        public Builder values(List<Float> values) {
            this.values = values;
            return this;
        }

        public SparseVector build() {
            return new SparseVector(indices, values);
        }
    }
}
