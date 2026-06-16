package com.limyedb.models;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.util.Map;

/**
 * Configuration for creating a new collection.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public class CollectionConfig {

    @JsonProperty("name")
    private String name;

    @JsonProperty("dimension")
    private int dimension;

    @JsonProperty("metric")
    private String metric;

    @JsonProperty("hnsw")
    private HNSWConfig hnsw;

    @JsonProperty("on_disk")
    private Boolean onDisk;

    @JsonProperty("vectors")
    private Map<String, VectorConfig> vectors;

    @JsonProperty("shard_count")
    private Integer shardCount;

    @JsonProperty("replication_factor")
    private Integer replicationFactor;

    public CollectionConfig() {
    }

    public CollectionConfig(String name, int dimension) {
        this.name = name;
        this.dimension = dimension;
        this.metric = "cosine";
    }

    public CollectionConfig(String name, int dimension, String metric) {
        this.name = name;
        this.dimension = dimension;
        this.metric = metric;
    }

    public String getName() {
        return name;
    }

    public CollectionConfig setName(String name) {
        this.name = name;
        return this;
    }

    public int getDimension() {
        return dimension;
    }

    public CollectionConfig setDimension(int dimension) {
        this.dimension = dimension;
        return this;
    }

    public String getMetric() {
        return metric;
    }

    public CollectionConfig setMetric(String metric) {
        this.metric = metric;
        return this;
    }

    public HNSWConfig getHnsw() {
        return hnsw;
    }

    public CollectionConfig setHnsw(HNSWConfig hnsw) {
        this.hnsw = hnsw;
        return this;
    }

    public Boolean getOnDisk() {
        return onDisk;
    }

    public CollectionConfig setOnDisk(Boolean onDisk) {
        this.onDisk = onDisk;
        return this;
    }

    public Map<String, VectorConfig> getVectors() {
        return vectors;
    }

    public CollectionConfig setVectors(Map<String, VectorConfig> vectors) {
        this.vectors = vectors;
        return this;
    }

    public Integer getShardCount() {
        return shardCount;
    }

    public CollectionConfig setShardCount(Integer shardCount) {
        this.shardCount = shardCount;
        return this;
    }

    public Integer getReplicationFactor() {
        return replicationFactor;
    }

    public CollectionConfig setReplicationFactor(Integer replicationFactor) {
        this.replicationFactor = replicationFactor;
        return this;
    }

    /**
     * Creates a builder for CollectionConfig.
     */
    public static Builder builder() {
        return new Builder();
    }

    public static class Builder {
        private String name;
        private int dimension;
        private String metric = "cosine";
        private HNSWConfig hnsw;
        private Boolean onDisk;
        private Map<String, VectorConfig> vectors;
        private Integer shardCount;
        private Integer replicationFactor;

        public Builder name(String name) {
            this.name = name;
            return this;
        }

        public Builder dimension(int dimension) {
            this.dimension = dimension;
            return this;
        }

        public Builder metric(String metric) {
            this.metric = metric;
            return this;
        }

        public Builder hnsw(HNSWConfig hnsw) {
            this.hnsw = hnsw;
            return this;
        }

        public Builder onDisk(boolean onDisk) {
            this.onDisk = onDisk;
            return this;
        }

        public Builder vectors(Map<String, VectorConfig> vectors) {
            this.vectors = vectors;
            return this;
        }

        public Builder shardCount(int shardCount) {
            this.shardCount = shardCount;
            return this;
        }

        public Builder replicationFactor(int replicationFactor) {
            this.replicationFactor = replicationFactor;
            return this;
        }

        public CollectionConfig build() {
            CollectionConfig config = new CollectionConfig();
            config.name = this.name;
            config.dimension = this.dimension;
            config.metric = this.metric;
            config.hnsw = this.hnsw;
            config.onDisk = this.onDisk;
            config.vectors = this.vectors;
            config.shardCount = this.shardCount;
            config.replicationFactor = this.replicationFactor;
            return config;
        }
    }

    /**
     * Configuration for a named vector.
     */
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public static class VectorConfig {
        @JsonProperty("dimension")
        private int dimension;

        @JsonProperty("metric")
        private String metric;

        @JsonProperty("hnsw")
        private HNSWConfig hnsw;

        @JsonProperty("on_disk")
        private Boolean onDisk;

        public VectorConfig() {
        }

        public VectorConfig(int dimension, String metric) {
            this.dimension = dimension;
            this.metric = metric;
        }

        public int getDimension() {
            return dimension;
        }

        public VectorConfig setDimension(int dimension) {
            this.dimension = dimension;
            return this;
        }

        public String getMetric() {
            return metric;
        }

        public VectorConfig setMetric(String metric) {
            this.metric = metric;
            return this;
        }

        public HNSWConfig getHnsw() {
            return hnsw;
        }

        public VectorConfig setHnsw(HNSWConfig hnsw) {
            this.hnsw = hnsw;
            return this;
        }

        public Boolean getOnDisk() {
            return onDisk;
        }

        public VectorConfig setOnDisk(Boolean onDisk) {
            this.onDisk = onDisk;
            return this;
        }
    }
}
