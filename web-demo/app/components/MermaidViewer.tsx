'use client';

import { useEffect, useRef, useState } from 'react';
import mermaid from 'mermaid';

interface MermaidViewerProps {
  exampleName: string;
}

// Pre-generated example diagrams
const examples: Record<string, { diagram: string; description: string; details: string }> = {
  tinyshop: {
    diagram: `flowchart TD
  subgraph changed["🔴 Changed Code"]
    discount["DiscountEngine.apply_discount()"]:::riskHigh
  end

  subgraph affected["⚠️ Blast Radius"]
    order["OrderService.price()"]:::riskMed
  end

  discount --> order

  classDef riskCrit fill:#7f1d1d,stroke:#fecaca,color:#fff,stroke-width:2px
  classDef riskHigh fill:#c2410c,stroke:#fed7aa,color:#fff,stroke-width:2px
  classDef riskMed fill:#a16207,stroke:#fef08a,color:#fff,stroke-width:2px
  classDef riskLow fill:#166534,stroke:#bbf7d0,color:#fff`,
    description: "E-commerce Service: Pricing Change Impact",
    details: "Adding a 'currency' parameter to apply_discount() affects OrderService.price() but NOT InventoryService.reserve() (different dependency path)"
  },

  api: {
    diagram: `flowchart TD
  subgraph changed["🔴 Changed Code"]
    auth["AuthMiddleware.validateToken()"]:::riskCrit
  end

  subgraph affected["⚠️ Blast Radius"]
    users["UserController.getProfile()"]:::riskHigh
    posts["PostController.create()"]:::riskHigh
    admin["AdminController.deleteUser()"]:::riskCrit
  end

  auth --> users
  auth --> posts
  auth --> admin

  classDef riskCrit fill:#7f1d1d,stroke:#fecaca,color:#fff,stroke-width:2px
  classDef riskHigh fill:#c2410c,stroke:#fed7aa,color:#fff,stroke-width:2px
  classDef riskMed fill:#a16207,stroke:#fef08a,color:#fff,stroke-width:2px
  classDef riskLow fill:#166534,stroke:#bbf7d0,color:#fff`,
    description: "API Service: Auth Middleware Change",
    details: "Modifying token validation affects all authenticated routes. AdminController is marked critical due to high privilege level."
  },

  database: {
    diagram: `flowchart TD
  subgraph changed["🔴 Changed Code"]
    query["QueryBuilder.buildSelect()"]:::riskHigh
  end

  subgraph layer1["Layer 1: Direct Callers"]
    repo["UserRepository.findAll()"]:::riskMed
    cache["CacheService.warmup()"]:::riskLow
  end

  subgraph layer2["Layer 2: Indirect Impact"]
    api["UserAPI.list()"]:::riskMed
    batch["BatchProcessor.sync()"]:::riskLow
  end

  query --> repo
  query --> cache
  repo --> api
  cache --> batch

  classDef riskCrit fill:#7f1d1d,stroke:#fecaca,color:#fff,stroke-width:2px
  classDef riskHigh fill:#c2410c,stroke:#fed7aa,color:#fff,stroke-width:2px
  classDef riskMed fill:#a16207,stroke:#fef08a,color:#fff,stroke-width:2px
  classDef riskLow fill:#166534,stroke:#bbf7d0,color:#fff`,
    description: "Database Layer: Query Builder Refactor",
    details: "Multi-layer impact analysis showing both direct callers and downstream effects. Severity decreases with distance."
  },

  microservices: {
    diagram: `flowchart LR
  subgraph changed["🔴 Changed: Payment Service"]
    payment["PaymentService.charge()"]:::riskCrit
  end

  subgraph affected1["⚠️ Order Service"]
    checkout["OrderService.checkout()"]:::riskHigh
    refund["OrderService.refund()"]:::riskMed
  end

  subgraph affected2["⚠️ Notification Service"]
    email["EmailService.sendReceipt()"]:::riskLow
  end

  subgraph affected3["⚠️ Analytics Service"]
    tracker["AnalyticsService.trackPurchase()"]:::riskLow
  end

  payment --> checkout
  payment --> refund
  checkout --> email
  checkout --> tracker

  classDef riskCrit fill:#7f1d1d,stroke:#fecaca,color:#fff,stroke-width:2px
  classDef riskHigh fill:#c2410c,stroke:#fed7aa,color:#fff,stroke-width:2px
  classDef riskMed fill:#a16207,stroke:#fef08a,color:#fff,stroke-width:2px
  classDef riskLow fill:#166534,stroke:#bbf7d0,color:#fff`,
    description: "Microservices: Cross-Service Dependencies",
    details: "Payment service change ripples through Order, Notification, and Analytics services. Graph shows RPC/HTTP call boundaries."
  }
};

export default function MermaidViewer({ exampleName }: MermaidViewerProps) {
  const mermaidRef = useRef<HTMLDivElement>(null);
  const [isLoading, setIsLoading] = useState(true);
  const example = examples[exampleName] || examples.tinyshop;

  useEffect(() => {
    mermaid.initialize({
      startOnLoad: false,
      theme: 'default',
      securityLevel: 'loose',
      fontFamily: 'system-ui, -apple-system, sans-serif',
    });
  }, []);

  useEffect(() => {
    const renderDiagram = async () => {
      if (!mermaidRef.current) return;

      setIsLoading(true);

      try {
        // Clear previous diagram
        mermaidRef.current.innerHTML = '';

        // Generate unique ID for this diagram
        const id = `mermaid-${Date.now()}`;

        // Render the diagram
        const { svg } = await mermaid.render(id, example.diagram);
        mermaidRef.current.innerHTML = svg;
      } catch (error) {
        console.error('Mermaid rendering error:', error);
        mermaidRef.current.innerHTML = '<p class="text-red-600">Error rendering diagram</p>';
      } finally {
        setIsLoading(false);
      }
    };

    renderDiagram();
  }, [exampleName, example.diagram]);

  return (
    <div className="space-y-4">
      {/* Example Description */}
      <div className="p-4 bg-blue-50 dark:bg-blue-900/20 border-l-4 border-blue-500 rounded">
        <h3 className="font-semibold text-blue-900 dark:text-blue-200 mb-2">
          {example.description}
        </h3>
        <p className="text-sm text-blue-800 dark:text-blue-300">
          {example.details}
        </p>
      </div>

      {/* Diagram Container */}
      <div className="relative">
        {isLoading && (
          <div className="absolute inset-0 flex items-center justify-center bg-white/50 dark:bg-gray-800/50 rounded-lg">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
          </div>
        )}

        <div
          ref={mermaidRef}
          className="mermaid min-h-[400px] flex items-center justify-center overflow-x-auto"
        />
      </div>

      {/* Legend */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 p-4 bg-gray-100 dark:bg-gray-800 rounded-lg">
        <div className="flex items-center gap-2">
          <div className="w-4 h-4 rounded" style={{ backgroundColor: '#7f1d1d' }}></div>
          <span className="text-sm font-medium">Critical Risk</span>
        </div>
        <div className="flex items-center gap-2">
          <div className="w-4 h-4 rounded" style={{ backgroundColor: '#c2410c' }}></div>
          <span className="text-sm font-medium">High Risk</span>
        </div>
        <div className="flex items-center gap-2">
          <div className="w-4 h-4 rounded" style={{ backgroundColor: '#a16207' }}></div>
          <span className="text-sm font-medium">Medium Risk</span>
        </div>
        <div className="flex items-center gap-2">
          <div className="w-4 h-4 rounded" style={{ backgroundColor: '#166534' }}></div>
          <span className="text-sm font-medium">Low Risk</span>
        </div>
      </div>

      {/* Analysis Stats */}
      <div className="grid grid-cols-3 gap-4 text-center">
        <div className="p-4 bg-white dark:bg-gray-800 rounded-lg shadow">
          <div className="text-2xl font-bold text-blue-600">
            {example.diagram.match(/-->/g)?.length || 0}
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400">Dependencies</div>
        </div>
        <div className="p-4 bg-white dark:bg-gray-800 rounded-lg shadow">
          <div className="text-2xl font-bold text-purple-600">
            {example.diagram.match(/subgraph/g)?.length || 0}
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400">Layers</div>
        </div>
        <div className="p-4 bg-white dark:bg-gray-800 rounded-lg shadow">
          <div className="text-2xl font-bold text-green-600">100%</div>
          <div className="text-sm text-gray-600 dark:text-gray-400">Confidence</div>
        </div>
      </div>
    </div>
  );
}
