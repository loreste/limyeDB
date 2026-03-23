import requests
import json
import time

def main():
    print("Testing collection...")
    # Health
    try:
        requests.get("http://localhost:8080/health")
    except:
        print("Server not running.")
        return

    requests.delete("http://localhost:8080/collections/trace_col")
    requests.post("http://localhost:8080/collections", json={"name": "trace_col", "dimension": 2, "metric": "cosine"})
    
    requests.put("http://localhost:8080/collections/trace_col/points", json={
        "points": [
            {"id": "a", "vector": [1.0, 1.0]},
            {"id": "b", "vector": [-1.0, -1.0]},
            {"id": "c", "vector": [1.0, -1.0]}
        ]
    })
    
    # Discovery payload EXACTLY as models.py output
    payload = {
        "target": [1.0, -1.0],
        "context": {
            "positive": [{"id": "a"}],
            "negative": [{"id": "b"}]
        },
        "limit": 2
    }
    print(f"Sending standard discovery: {json.dumps(payload)}")
    res = requests.post("http://localhost:8080/collections/trace_col/discover", json=payload)
    print("STATUS:", res.status_code)
    print("BODY:", res.text)
    
if __name__ == "__main__":
    main()
