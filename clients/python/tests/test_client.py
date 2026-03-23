import time
import pytest
import subprocess
import requests

from limyedb.client import LimyeDBClient
from limyedb.models import Point, CollectionConfig, DiscoverParams, ContextPair, ContextExample

@pytest.fixture(scope="module")
def limyedb_server():
    # Attempt to ping first
    try:
        if requests.get("http://localhost:8080/health").status_code == 200:
            yield
            return
    except requests.exceptions.ConnectionError:
        pass
        
    # Start server if not running
    proc = subprocess.Popen(["go", "run", "cmd/limyedb/main.go"], cwd="../..")
    
    # Wait for ready
    for _ in range(30):
        try:
            if requests.get("http://localhost:8080/health").status_code == 200:
                break
        except requests.exceptions.ConnectionError:
            time.sleep(1)
            
    yield
    proc.terminate()
    proc.wait()

def test_collections(limyedb_server):
    client = LimyeDBClient()
    
    # Clean up
    try:
        client.delete_collection("test_col")
    except:
        pass
        
    cfg = CollectionConfig(name="test_col", dimension=4, metric="cosine")
    res = client.create_collection(cfg)
    assert res["success"] is True
    
    cols = client.list_collections()
    assert any(c["name"] == "test_col" for c in cols["collections"])
    
    client.delete_collection("test_col")

def test_upsert_and_search(limyedb_server):
    client = LimyeDBClient()
    
    try:
        client.delete_collection("search_col")
    except:
        pass
        
    client.create_collection(CollectionConfig(name="search_col", dimension=3, metric="euclidean"))
    
    points = [
        Point(id="1", vector=[1.0, 0.0, 0.0], payload={"color": "red"}),
        Point(id="2", vector=[0.0, 1.0, 0.0], payload={"color": "green"}),
        Point(id="3", vector=[0.0, 0.0, 1.0], payload={"color": "blue"}),
    ]
    
    client.upsert("search_col", points)
    
    # Eventual consistency or direct? LimyeDB is real-time graph! 
    # Just query immediately.
    matches = client.search("search_col", vector=[1.0, 0.1, 0.0], limit=2)
    
    assert len(matches) == 2
    assert matches[0].id == "1"
    assert matches[0].payload["color"] == "red"

def test_discover(limyedb_server):
    client = LimyeDBClient()
    
    try:
        client.delete_collection("disc_col")
    except:
        pass
        
    client.create_collection(CollectionConfig(name="disc_col", dimension=2))
    points = [
        Point(id="a", vector=[1.0, 1.0]),
        Point(id="b", vector=[-1.0, -1.0]),
        Point(id="c", vector=[1.0, -1.0]),
    ]
    client.upsert("disc_col", points)
    
    # Discover away from negative B, towards positive A
    params = DiscoverParams(
        context=ContextPair(
            positive=[ContextExample(id="a")],
            negative=[ContextExample(id="b")]
        ), 
        limit=2
    )
    search_matches = client.search("disc_col", vector=[1.0, -1.0], limit=3)
    print("STANDARD SEARCH ON DISC_COL:", search_matches)
    
    matches = client.discover("disc_col", params)
    print("DISCOVERY ON DISC_COL:", matches)
    assert matches[0].id == "a"
