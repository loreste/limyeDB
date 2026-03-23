import requests
from typing import List, Dict, Any, Optional

from .models import Point, CollectionConfig, Match, DiscoverParams

class LimyeDBClient:
    """Official Python Client for LimyeDB"""
    
    def __init__(self, host: str = "http://localhost:8080"):
        self.host = host.rstrip('/')
        
    def _post(self, path: str, json: dict = None) -> dict:
        resp = requests.post(f"{self.host}{path}", json=json)
        resp.raise_for_status()
        return resp.json()
        
    def _put(self, path: str, json: dict = None) -> dict:
        resp = requests.put(f"{self.host}{path}", json=json)
        resp.raise_for_status()
        return resp.json()
        
    def _get(self, path: str) -> dict:
        resp = requests.get(f"{self.host}{path}")
        resp.raise_for_status()
        return resp.json()
        
    def _delete(self, path: str) -> dict:
        resp = requests.delete(f"{self.host}{path}")
        resp.raise_for_status()
        return resp.json()
        
    # Collections API
    
    def create_collection(self, config: CollectionConfig) -> dict:
        return self._post("/collections", json=config.model_dump(exclude_none=True))
        
    def list_collections(self) -> dict:
        return self._get("/collections")
        
    def get_collection(self, name: str) -> dict:
        return self._get(f"/collections/{name}")
        
    def delete_collection(self, name: str) -> dict:
        return self._delete(f"/collections/{name}")
        
    # Vectors API
    
    def upsert(self, collection_name: str, points: List[Point]) -> dict:
        payload = {"points": [p.model_dump(exclude_none=True) for p in points]}
        return self._put(f"/collections/{collection_name}/points", json=payload)
        
    def delete(self, collection_name: str, ids: List[str]) -> dict:
        return self._post(f"/collections/{collection_name}/points/delete", json={"ids": ids})
        
    def search(self, 
               collection_name: str, 
               vector: List[float], 
               limit: int = 10,
               filter: Optional[Dict[str, Any]] = None,
               ef: int = 100,
               with_payload: bool = True,
               with_vector: bool = False) -> List[Match]:
               
        payload = {
            "vector": vector,
            "limit": limit,
            "ef": ef,
            "with_payload": with_payload,
            "with_vector": with_vector
        }
        if filter:
            payload["filter"] = filter
            
        res = self._post(f"/collections/{collection_name}/search", json=payload)
        return [Match(**hit) for hit in res.get("result", [])]
        
    def discover(self, collection_name: str, params: DiscoverParams) -> List[Match]:
        res = self._post(f"/collections/{collection_name}/discover", json=params.model_dump(exclude_none=True))
        return [Match(**hit) for hit in res.get("points", [])]
