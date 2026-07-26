'use client';

import { useState } from 'react';
import MermaidViewer from './components/MermaidViewer';
import ExampleSelector from './components/ExampleSelector';

export default function Home() {
    const [selectedExample, setSelectedExample] = useState('tinyshop');
    const [showArchitecture, setShowArchitecture] = useState(false);

    return (
        <div className="min-h-screen p-8 pb-20 gap-16 sm:p-20">
            <main className="max-w-6xl mx-auto">
                {/* Header */}
                <div className="text-center mb-12">
                    <h1 className="text-5xl font-bold mb-4 bg-gradient-to-r from-blue-600 to-purple-600 bg-clip-text text-transparent">
                        PR Blast Radius Analyzer
                    </h1>
                    <p className="text-xl text-gray-600 dark:text-gray-400 mb-6">
                        Visualize the impact of code changes with tree-sitter AST parsing + LLM visualization
                    </p>

                    {/* Tech Stack Badges */}
                    <div className="flex flex-wrap gap-2 justify-center mb-8">
                        <span className="px-3 py-1 bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200 rounded-full text-sm font-medium">
                            Go
                        </span>
                        <span className="px-3 py-1 bg-purple-100 dark:bg-purple-900 text-purple-800 dark:text-purple-200 rounded-full text-sm font-medium">
                            Python
                        </span>
                        <span className="px-3 py-1 bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200 rounded-full text-sm font-medium">
                            Tree-sitter
                        </span>
                        <span className="px-3 py-1 bg-orange-100 dark:bg-orange-900 text-orange-800 dark:text-orange-200 rounded-full text-sm font-medium">
                            Pydantic
                        </span>
                        <span className="px-3 py-1 bg-pink-100 dark:bg-pink-900 text-pink-800 dark:text-pink-200 rounded-full text-sm font-medium">
                            Mermaid.js
                        </span>
                    </div>

                    {/* CTA Buttons */}
                    <div className="flex gap-4 justify-center">
                        <a
                            href="https://github.com/yourusername/pr-blast-radius"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="px-6 py-3 bg-gray-900 dark:bg-white text-white dark:text-gray-900 rounded-lg font-medium hover:bg-gray-700 dark:hover:bg-gray-200 transition-colors"
                        >
                            View on GitHub
                        </a>
                        <button
                            onClick={() => setShowArchitecture(!showArchitecture)}
                            className="px-6 py-3 bg-blue-600 text-white rounded-lg font-medium hover:bg-blue-700 transition-colors"
                        >
                            {showArchitecture ? 'Hide' : 'Show'} Architecture
                        </button>
                    </div>
                </div>

                {/* Architecture Diagram */}
                {showArchitecture && (
                    <div className="mb-12 p-6 bg-white dark:bg-gray-800 rounded-lg shadow-lg">
                        <h2 className="text-2xl font-bold mb-4">How It Works</h2>

                        <div className="grid md:grid-cols-2 gap-6">
                            <div>
                                <h3 className="text-lg font-semibold mb-2 text-blue-600">
                                    Phase A: Static Analysis (Go)
                                </h3>
                                <ul className="space-y-2 text-sm text-gray-700 dark:text-gray-300 list-disc list-inside">
                                    <li>Tree-sitter AST parsing (Go + Python)</li>
                                    <li>Git diff extraction (go-git)</li>
                                    <li>Symbol table construction</li>
                                    <li>Confidence-scored edge resolution (R1–R7, P1–P8)</li>
                                    <li>Reverse BFS traversal with cycle detection</li>
                                    <li>Severity scoring</li>
                                </ul>
                            </div>

                            <div>
                                <h3 className="text-lg font-semibold mb-2 text-purple-600">
                                    Phase B: Visualization (Python + LLM)
                                </h3>
                                <ul className="space-y-2 text-sm text-gray-700 dark:text-gray-300 list-disc list-inside">
                                    <li>LLM generates structured Pydantic specification</li>
                                    <li>Schema validation prevents malformed output</li>
                                    <li>Deterministic compiler converts specification to diagram text</li>
                                    <li>Three-tier fallback pipeline: Gemini → Ollama → Template</li>
                                    <li>Guaranteed output via template fallback</li>
                                </ul>
                            </div>
                        </div>

                        <div className="mt-4 p-4 bg-yellow-50 dark:bg-yellow-900/20 border-l-4 border-yellow-500 rounded">
                            <p className="text-sm font-semibold text-yellow-800 dark:text-yellow-200">
                                <strong>Key Innovation:</strong> LLM responsible for presentation; correctness enforced by deterministic analysis and schema validation.
                            </p>
                        </div>
                    </div>
                )}

                {/* Example Selector */}
                <ExampleSelector
                    selectedExample={selectedExample}
                    onSelect={setSelectedExample}
                />

                {/* Mermaid Diagram Viewer */}
                <div className="mt-8">
                    <MermaidViewer exampleName={selectedExample} />
                </div>

                {/* Features Grid */}
                <div className="mt-16 grid md:grid-cols-3 gap-8">
                    <div className="p-6 bg-white dark:bg-gray-800 rounded-lg shadow-lg">
                        <div className="text-3xl mb-3">🎯</div>
                        <h3 className="text-xl font-semibold mb-2">100% Deterministic</h3>
                        <p className="text-gray-600 dark:text-gray-400 text-sm">
                            Go-powered AST parsing with confidence scores. Unresolvable calls produce zero edges, never guesses.
                        </p>
                    </div>

                    <div className="p-6 bg-white dark:bg-gray-800 rounded-lg shadow-lg">
                        <div className="text-3xl mb-3">🔒</div>
                        <h3 className="text-xl font-semibold mb-2">Schema-Safe LLM</h3>
                        <p className="text-gray-600 dark:text-gray-400 text-sm">
                            Pydantic validation ensures LLM output is always valid. Structured generation, not prompt engineering.
                        </p>
                    </div>

                    <div className="p-6 bg-white dark:bg-gray-800 rounded-lg shadow-lg">
                        <div className="text-3xl mb-3">⚡</div>
                        <h3 className="text-xl font-semibold mb-2">Multi-Language</h3>
                        <p className="text-gray-600 dark:text-gray-400 text-sm">
                            Supports Go and Python with tree-sitter. Extensible to TypeScript, Rust, Java via parser plugins.
                        </p>
                    </div>
                </div>

                {/* Installation Instructions */}
                <div className="mt-16 p-6 bg-gradient-to-r from-blue-50 to-purple-50 dark:from-gray-800 dark:to-gray-800 rounded-lg">
                    <h2 className="text-2xl font-bold mb-4">Try It Yourself</h2>
                    <div className="space-y-4">
                        <div>
                            <h3 className="font-semibold mb-2">Via Docker:</h3>
                            <pre className="bg-gray-900 text-gray-100 p-4 rounded overflow-x-auto text-sm">
                                <code>docker pull yourname/pr-blast-radius:latest{'\n'}
                                    docker run -v $(pwd):/repo pr-blast-radius --repo /repo --diff WORKING_TREE</code>
                            </pre>
                        </div>

                        <div>
                            <h3 className="font-semibold mb-2">Via GitHub:</h3>
                            <pre className="bg-gray-900 text-gray-100 p-4 rounded overflow-x-auto text-sm">
                                <code>git clone https://github.com/yourusername/pr-blast-radius{'\n'}
                                    cd pr-blast-radius{'\n'}
                                    make install  # Builds Go binary + installs Python deps{'\n'}
                                    python -m orchestrator.cli scan --repo ./my-repo --diff HEAD~1</code>
                            </pre>
                        </div>
                    </div>
                </div>

                {/* Footer */}
                <div className="mt-16 text-center text-sm text-gray-500 dark:text-gray-400">
                    <p>Built with Next.js, deployed on Vercel</p>
                    <p className="mt-2">
                        <a href="https://github.com/yourusername/pr-blast-radius" className="hover:text-blue-600 transition-colors">
                            Star on GitHub
                        </a>
                        {' • '}
                        <a href="https://twitter.com/yourusername" className="hover:text-blue-600 transition-colors">
                            Follow on Twitter
                        </a>
                    </p>
                </div>
            </main>
        </div>
    );
}
