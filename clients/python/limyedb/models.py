from typing import List, Dict, Any, Optional, Union
from pydantic import BaseModel, Field

class HNSWConfig(BaseModel):
    m: Optional[int] = Field(default=None, alias="m")
    ef_construction: Optional[int] = Field(default=None, alias="ef_construction")
    ef_search: Optional[int] = Field(default=None, alias="ef_search")

class CollectionConfig(BaseModel):
    name: str
    dimension: int
    metric: Optional[str] = "cosine"
    on_disk: Optional[bool] = False
    hnsw: Optional[HNSWConfig] = None

class Point(BaseModel):
    id: str
    vector: List[float]
    payload: Optional[Dict[str, Any]] = None

class SearchParams(BaseModel):
    vector: List[float]
    limit: Optional[int] = 10
    ef: Optional[int] = 100
    filter: Optional[Dict[str, Any]] = None
    with_vector: Optional[bool] = False
    with_payload: Optional[bool] = True

class Match(BaseModel):
    id: str
    score: float
    vector: Optional[List[float]] = None
    payload: Optional[Dict[str, Any]] = None

class ContextExample(BaseModel):
    id: Optional[str] = None
    vector: Optional[List[float]] = None

    def _post(self, path: str, json: dict = None) -> dict:
        resp = requests.post(f"{self.host}{path}", json=json)
        if not resp.ok:
            print(f"ERROR on _post {path}: {resp.text}")
        resp.raise_for_status()
        return resp.json()

class ContextPair(BaseModel):
    positive: List[ContextExample]
    negative: Optional[List[ContextExample]] = None

class DiscoverParams(BaseModel):
    target: Optional[List[float]] = None
    context: Optional[ContextPair] = None
    limit: Optional[int] = 10
    ef: Optional[int] = 100
    filter: Optional[Dict[str, Any]] = None
    with_vector: Optional[bool] = False
    with_payload: Optional[bool] = True
