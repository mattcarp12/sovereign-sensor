import { useState, useEffect } from 'react';
import { ComposableMap, Geographies, Geography } from '@vnedyalk0v/react19-simple-maps';
import axios from 'axios';

// Standard TopoJSON for the world map
const geoUrl = "https://unpkg.com/world-atlas@2.0.2/countries-110m.json";

function App() {
  const [policies, setPolicies] = useState([]);
  const [loading, setLoading] = useState(true);

  const fetchPolicies = async () => {
    try {
      const response = await axios.get('/api/policies');
      setPolicies(response.data || []);
    } catch (error) {
      console.error("Failed to fetch policies:", error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchPolicies();
  }, []);

  return (
    <div className="min-h-screen bg-white text-black p-8 font-sans">

      {/* Header */}
      <header className="mb-8 border-b-2 border-black pb-4 flex justify-between items-end">
        <div>
          <h1 className="text-4xl font-bold tracking-tight">Sovereign Sensor</h1>
          <p className="text-gray-500 mt-1">Data Plane & Policy Overview</p>
        </div>
        <div className="text-sm font-mono bg-black text-white px-3 py-1">
          STATUS: {loading ? 'SYNCING...' : 'LIVE'}
        </div>
      </header>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">

        {/* Left Column: Policy CRUD */}
        <div className="lg:col-span-1 border-2 border-black p-6">
          <div className="flex justify-between items-center mb-6">
            <h2 className="text-2xl font-bold">Active Policies</h2>
            <button className="bg-[#326CE5] hover:bg-blue-700 text-white font-bold py-2 px-4 text-sm transition-colors">
              + NEW POLICY
            </button>
          </div>

          {loading ? (
            <p className="font-mono text-sm animate-pulse">Loading rules from Kubernetes API...</p>
          ) : policies.length === 0 ? (
            <p className="text-gray-500 font-mono text-sm">No custom policies applied.</p>
          ) : (
            <div className="space-y-4">
              {policies.map((policy) => (
                <div key={policy.metadata.uid} className="border border-gray-300 p-4">
                  <div className="flex justify-between items-start mb-2">
                    <h3 className="font-bold text-lg">{policy.metadata.name}</h3>
                    <span className={`px-2 py-1 text-xs font-mono text-white ${policy.spec.action === 'block' ? 'bg-black' : 'bg-[#326CE5]'}`}>
                      {policy.spec.action.toUpperCase()}
                    </span>
                  </div>
                  <p className="text-sm text-gray-600 mb-2">{policy.spec.description}</p>
                  <div className="text-xs font-mono text-gray-500">
                    <p>Namespaces: {policy.spec.namespaces?.join(', ')}</p>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Right Column: The Map */}
        <div className="lg:col-span-2 border-2 border-black p-6 flex flex-col">
          <h2 className="text-2xl font-bold mb-2">Global Network Topography</h2>
          <p className="text-gray-500 text-sm mb-4">Real-time packet destinations will be visualized here.</p>

          <div className="flex-grow bg-gray-50 border border-gray-200 overflow-hidden flex items-center justify-center">
            <ComposableMap projectionConfig={{ scale: 140 }} width={800} height={400} style={{ width: "100%", height: "auto" }}>
              <Geographies geography={geoUrl}>
                {({ geographies }) =>
                  geographies.map((geo) => (
                    <Geography
                      key={geo.rsmKey}
                      geography={geo}
                      fill="#EAEAEC"
                      stroke="#D6D6DA"
                      strokeWidth={0.5}
                      style={{
                        default: { outline: "none" },
                        hover: { fill: "#326CE5", outline: "none" },
                        pressed: { fill: "#000000", outline: "none" },
                      }}
                    />
                  ))
                }
              </Geographies>
            </ComposableMap>
          </div>
        </div>

      </div>
    </div>
  );
}

export default App;