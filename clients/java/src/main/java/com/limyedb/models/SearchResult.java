package com.limyedb.models;

import com.fasterxml.jackson.annotation.JsonProperty;

import java.util.List;
import java.util.Map;

/**
 * Represents a search result.
 */
public class SearchResult {

    @JsonProperty("id")
    private String id;

    @JsonProperty("score")
    private float score;

    @JsonProperty("vector")
    private List<Float> vector;

    @JsonProperty("payload")
    private Map<String, Object> payload;

    @JsonProperty("vector_name")
    private String vectorName;

    public SearchResult() {
    }

    public String getId() {
        return id;
    }

    public SearchResult setId(String id) {
        this.id = id;
        return this;
    }

    public float getScore() {
        return score;
    }

    public SearchResult setScore(float score) {
        this.score = score;
        return this;
    }

    public List<Float> getVector() {
        return vector;
    }

    public SearchResult setVector(List<Float> vector) {
        this.vector = vector;
        return this;
    }

    public Map<String, Object> getPayload() {
        return payload;
    }

    public SearchResult setPayload(Map<String, Object> payload) {
        this.payload = payload;
        return this;
    }

    public String getVectorName() {
        return vectorName;
    }

    public SearchResult setVectorName(String vectorName) {
        this.vectorName = vectorName;
        return this;
    }

    @Override
    public String toString() {
        return "SearchResult{" +
                "id='" + id + '\'' +
                ", score=" + score +
                ", payload=" + payload +
                '}';
    }
}
