package com.limyedb.models;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;

/**
 * HNSW index configuration.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public class HNSWConfig {

    @JsonProperty("m")
    private Integer m;

    @JsonProperty("ef_construction")
    private Integer efConstruction;

    @JsonProperty("ef_search")
    private Integer efSearch;

    @JsonProperty("max_elements")
    private Integer maxElements;

    public HNSWConfig() {
    }

    public HNSWConfig(Integer m, Integer efConstruction, Integer efSearch) {
        this.m = m;
        this.efConstruction = efConstruction;
        this.efSearch = efSearch;
    }

    public Integer getM() {
        return m;
    }

    public HNSWConfig setM(Integer m) {
        this.m = m;
        return this;
    }

    public Integer getEfConstruction() {
        return efConstruction;
    }

    public HNSWConfig setEfConstruction(Integer efConstruction) {
        this.efConstruction = efConstruction;
        return this;
    }

    public Integer getEfSearch() {
        return efSearch;
    }

    public HNSWConfig setEfSearch(Integer efSearch) {
        this.efSearch = efSearch;
        return this;
    }

    public Integer getMaxElements() {
        return maxElements;
    }

    public HNSWConfig setMaxElements(Integer maxElements) {
        this.maxElements = maxElements;
        return this;
    }

    /**
     * Creates a builder for HNSWConfig.
     */
    public static Builder builder() {
        return new Builder();
    }

    public static class Builder {
        private Integer m;
        private Integer efConstruction;
        private Integer efSearch;
        private Integer maxElements;

        public Builder m(int m) {
            this.m = m;
            return this;
        }

        public Builder efConstruction(int efConstruction) {
            this.efConstruction = efConstruction;
            return this;
        }

        public Builder efSearch(int efSearch) {
            this.efSearch = efSearch;
            return this;
        }

        public Builder maxElements(int maxElements) {
            this.maxElements = maxElements;
            return this;
        }

        public HNSWConfig build() {
            HNSWConfig config = new HNSWConfig();
            config.m = this.m;
            config.efConstruction = this.efConstruction;
            config.efSearch = this.efSearch;
            config.maxElements = this.maxElements;
            return config;
        }
    }
}
