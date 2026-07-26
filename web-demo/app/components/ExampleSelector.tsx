'use client';

interface ExampleSelectorProps {
  selectedExample: string;
  onSelect: (example: string) => void;
}

const examples = [
  {
    id: 'tinyshop',
    icon: '🛒',
    title: 'E-commerce',
    subtitle: 'Pricing function change'
  },
  {
    id: 'api',
    icon: '🔐',
    title: 'API Service',
    subtitle: 'Auth middleware update'
  },
  {
    id: 'database',
    icon: '🗄️',
    title: 'Database Layer',
    subtitle: 'Query builder refactor'
  },
  {
    id: 'microservices',
    icon: '🔗',
    title: 'Microservices',
    subtitle: 'Cross-service impact'
  }
];

export default function ExampleSelector({ selectedExample, onSelect }: ExampleSelectorProps) {
  return (
    <div>
      <h2 className="text-2xl font-bold mb-4 text-center">
        Interactive Examples
      </h2>
      <p className="text-center text-gray-600 dark:text-gray-400 mb-6">
        Click an example to see how different code changes propagate through your system
      </p>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {examples.map((example) => (
          <button
            key={example.id}
            onClick={() => onSelect(example.id)}
            className={`
              p-6 rounded-lg border-2 transition-all
              ${selectedExample === example.id
                ? 'border-blue-600 bg-blue-50 dark:bg-blue-900/20 shadow-lg scale-105'
                : 'border-gray-200 dark:border-gray-700 hover:border-blue-400 hover:shadow-md'
              }
            `}
          >
            <div className="text-4xl mb-2">{example.icon}</div>
            <div className="font-semibold mb-1">{example.title}</div>
            <div className="text-sm text-gray-600 dark:text-gray-400">
              {example.subtitle}
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}
