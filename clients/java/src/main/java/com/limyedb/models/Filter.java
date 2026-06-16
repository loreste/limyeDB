package com.limyedb.models;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.util.ArrayList;
import java.util.List;

/**
 * Represents a filter for search queries.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public class Filter {

    @JsonProperty("must")
    private List<Condition> must;

    @JsonProperty("must_not")
    private List<Condition> mustNot;

    @JsonProperty("should")
    private List<Condition> should;

    public Filter() {
    }

    public List<Condition> getMust() {
        return must;
    }

    public Filter setMust(List<Condition> must) {
        this.must = must;
        return this;
    }

    public Filter addMust(Condition condition) {
        if (this.must == null) {
            this.must = new ArrayList<>();
        }
        this.must.add(condition);
        return this;
    }

    public List<Condition> getMustNot() {
        return mustNot;
    }

    public Filter setMustNot(List<Condition> mustNot) {
        this.mustNot = mustNot;
        return this;
    }

    public Filter addMustNot(Condition condition) {
        if (this.mustNot == null) {
            this.mustNot = new ArrayList<>();
        }
        this.mustNot.add(condition);
        return this;
    }

    public List<Condition> getShould() {
        return should;
    }

    public Filter setShould(List<Condition> should) {
        this.should = should;
        return this;
    }

    public Filter addShould(Condition condition) {
        if (this.should == null) {
            this.should = new ArrayList<>();
        }
        this.should.add(condition);
        return this;
    }

    /**
     * Creates a builder for Filter.
     */
    public static Builder builder() {
        return new Builder();
    }

    public static class Builder {
        private List<Condition> must;
        private List<Condition> mustNot;
        private List<Condition> should;

        public Builder must(List<Condition> must) {
            this.must = must;
            return this;
        }

        public Builder addMust(Condition condition) {
            if (this.must == null) {
                this.must = new ArrayList<>();
            }
            this.must.add(condition);
            return this;
        }

        public Builder mustNot(List<Condition> mustNot) {
            this.mustNot = mustNot;
            return this;
        }

        public Builder addMustNot(Condition condition) {
            if (this.mustNot == null) {
                this.mustNot = new ArrayList<>();
            }
            this.mustNot.add(condition);
            return this;
        }

        public Builder should(List<Condition> should) {
            this.should = should;
            return this;
        }

        public Builder addShould(Condition condition) {
            if (this.should == null) {
                this.should = new ArrayList<>();
            }
            this.should.add(condition);
            return this;
        }

        public Filter build() {
            Filter filter = new Filter();
            filter.must = this.must;
            filter.mustNot = this.mustNot;
            filter.should = this.should;
            return filter;
        }
    }

    /**
     * Represents a filter condition.
     */
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public static class Condition {

        @JsonProperty("key")
        private String key;

        @JsonProperty("match")
        private Match match;

        @JsonProperty("range")
        private Range range;

        public Condition() {
        }

        /**
         * Creates a match condition.
         */
        public static Condition match(String key, Object value) {
            Condition condition = new Condition();
            condition.key = key;
            condition.match = new Match(value);
            return condition;
        }

        /**
         * Creates a range condition.
         */
        public static Condition range(String key, Double gt, Double gte, Double lt, Double lte) {
            Condition condition = new Condition();
            condition.key = key;
            condition.range = new Range(gt, gte, lt, lte);
            return condition;
        }

        public String getKey() {
            return key;
        }

        public Condition setKey(String key) {
            this.key = key;
            return this;
        }

        public Match getMatch() {
            return match;
        }

        public Condition setMatch(Match match) {
            this.match = match;
            return this;
        }

        public Range getRange() {
            return range;
        }

        public Condition setRange(Range range) {
            this.range = range;
            return this;
        }
    }

    /**
     * Match condition for exact value matching.
     */
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public static class Match {

        @JsonProperty("value")
        private Object value;

        public Match() {
        }

        public Match(Object value) {
            this.value = value;
        }

        public Object getValue() {
            return value;
        }

        public Match setValue(Object value) {
            this.value = value;
            return this;
        }
    }

    /**
     * Range condition for numeric comparisons.
     */
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public static class Range {

        @JsonProperty("gt")
        private Double gt;

        @JsonProperty("gte")
        private Double gte;

        @JsonProperty("lt")
        private Double lt;

        @JsonProperty("lte")
        private Double lte;

        public Range() {
        }

        public Range(Double gt, Double gte, Double lt, Double lte) {
            this.gt = gt;
            this.gte = gte;
            this.lt = lt;
            this.lte = lte;
        }

        public Double getGt() {
            return gt;
        }

        public Range setGt(Double gt) {
            this.gt = gt;
            return this;
        }

        public Double getGte() {
            return gte;
        }

        public Range setGte(Double gte) {
            this.gte = gte;
            return this;
        }

        public Double getLt() {
            return lt;
        }

        public Range setLt(Double lt) {
            this.lt = lt;
            return this;
        }

        public Double getLte() {
            return lte;
        }

        public Range setLte(Double lte) {
            this.lte = lte;
            return this;
        }

        /**
         * Creates a range condition for greater than.
         */
        public static Range gt(double value) {
            return new Range(value, null, null, null);
        }

        /**
         * Creates a range condition for greater than or equal.
         */
        public static Range gte(double value) {
            return new Range(null, value, null, null);
        }

        /**
         * Creates a range condition for less than.
         */
        public static Range lt(double value) {
            return new Range(null, null, value, null);
        }

        /**
         * Creates a range condition for less than or equal.
         */
        public static Range lte(double value) {
            return new Range(null, null, null, value);
        }

        /**
         * Creates a range condition for between (inclusive).
         */
        public static Range between(double min, double max) {
            return new Range(null, min, null, max);
        }
    }
}
