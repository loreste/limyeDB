from setuptools import setup, find_packages

setup(
    name="limyedb",
    version="0.1.0",
    description="Official Python Client and LangChain VectorStore for LimyeDB",
    author="LimyeDB Team",
    packages=find_packages(),
    install_requires=[
        "requests>=2.28.0",
        "pydantic>=2.0.0",
    ],
    extras_require={
        "langchain": ["langchain-core>=0.1.0"],
        "dev": ["pytest>=7.0.0", "requests-mock"]
    },
    python_requires=">=3.8",
)
