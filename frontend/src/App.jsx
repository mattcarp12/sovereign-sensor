import { useState, useEffect } from 'react';
import { ComposableMap, Geographies, Geography } from '@vnedyalk0v/react19-simple-maps';
import axios from 'axios';

const geoUrl = "https://unpkg.com/world-atlas@2.0.2/countries-110m.json";

function App() {
  const [policies, setPolicies] = useState([]);
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);

  // New Policy Form State
  const [newPolicy, setNewPolicy] = useState({
    name: '',
    namespace: 'default',
    country: '',
    action: 'block-kill'
  });

  // Fetch Active Policies
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

  // Fetch Live K8s Events (Violations)
  const fetchViolations = async () => {
    try {
      const response = await axios.get('/api/violations');
      setEvents((response.data || []).reverse()); // Newest first
    } catch (error) {
      console.error("Failed to fetch violations:", error);
    }
  };

  useEffect(() => {
    fetchPolicies();
    fetchViolations();

    // Poll for new blocked packets every 3 seconds
    const interval = setInterval(fetchViolations, 3000);
    return () => clearInterval(interval);
  }, []);

  // CRUD: Create
  const handleCreatePolicy = async (e) => {
    e.preventDefault();
    try {
      await axios.post('/api/policies', newPolicy);
      setShowForm(false);
      setNewPolicy({ name: '', namespace: 'default', country: '', action: 'block-kill' });
      fetchPolicies(); // Refresh list
    } catch (error) {
      console.error("Failed to create policy:", error);
      alert("Error creating policy. Check console.");
    }
  };

  // CRUD: Delete
  const handleDeletePolicy = async (policyName) => {
    try {
      await axios.delete(`/api/policies/${policyName}`);
      fetchPolicies(); // Refresh list
    } catch (error) {
      console.error("Failed to delete policy:", error);
    }
  };

  return (
    <div className="min-h-screen bg-white text-black p-8 font-sans">

      {/* Header */}
      <header className="mb-8 border-b-2 border-black pb-4 flex justify-between items-end">
        <div>
          <h1 className="text-4xl font-bold tracking-tight">Sovereign Sensor</h1>
          <p className="text-gray-500 mt-1">Data Plane & Policy Overview</p>
        </div>
        <div className="text-sm font-mono bg-black text-white px-3 py-1 flex items-center gap-2">
          <div className="w-2 h-2 rounded-full bg-green-500 animate-pulse"></div>
          {loading ? 'SYNCING...' : 'LIVE'}
        </div>
      </header>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">

        {/* Left Column: Policy Management */}
        <div className="lg:col-span-1 flex flex-col gap-6">

          {/* Policy List Panel */}
          <div className="border-2 border-black p-6">
            <div className="flex justify-between items-center mb-6">
              <h2 className="text-2xl font-bold">Active Policies</h2>
              <button
                onClick={() => setShowForm(!showForm)}
                className="bg-black hover:bg-gray-800 text-white font-bold py-2 px-4 text-sm transition-colors"
              >
                {showForm ? 'CANCEL' : '+ NEW POLICY'}
              </button>
            </div>

            {/* Create Policy Form */}
            {showForm && (
              <form onSubmit={handleCreatePolicy} className="mb-6 p-4 bg-gray-100 border border-black text-sm">
                <div className="mb-3">
                  <label className="block font-bold mb-1">Policy Name</label>
                  <input required type="text" className="w-full p-2 border border-gray-400" placeholder="e.g., block-ru"
                    value={newPolicy.name} onChange={e => setNewPolicy({ ...newPolicy, name: e.target.value })} />
                </div>
                <div className="mb-3 flex gap-2">
                  <div className="flex-1">
                    <label className="block font-bold mb-1">Target Namespace</label>
                    <input required type="text" className="w-full p-2 border border-gray-400"
                      value={newPolicy.namespace} onChange={e => setNewPolicy({ ...newPolicy, namespace: e.target.value })} />
                  </div>
                  <div className="flex-1">
                    <label className="block font-bold mb-1">Target Country (ISO)</label>
                    <input required type="text" className="w-full p-2 border border-gray-400" placeholder="e.g., RU" maxLength="2"
                      value={newPolicy.country} onChange={e => setNewPolicy({ ...newPolicy, country: e.target.value.toUpperCase() })} />
                  </div>
                </div>
                <button type="submit" className="w-full bg-[#326CE5] hover:bg-blue-700 text-white font-bold py-2">
                  DEPLOY TO MESH
                </button>
              </form>
            )}

            {/* Policy List */}
            {loading ? (
              <p className="font-mono text-sm animate-pulse">Loading rules from Kubernetes API...</p>
            ) : policies.length === 0 ? (
              <p className="text-gray-500 font-mono text-sm">No custom policies applied.</p>
            ) : (
              <div className="space-y-4">
                {policies.map((policy) => {
                  // Safely extract data depending on your Go struct serialization
                  const actions = policy.spec?.actions || [];
                  const disallowed = policy.spec?.disallowedCountries || [];

                  return (
                    <div key={policy.metadata?.uid} className="border border-gray-300 p-4 relative group">
                      <button
                        onClick={() => handleDeletePolicy(policy.metadata?.name)}
                        className="absolute top-2 right-2 text-red-500 font-bold text-xs opacity-0 group-hover:opacity-100 transition-opacity"
                      >
                        [DELETE]
                      </button>
                      <div className="flex justify-between items-start mb-2">
                        <h3 className="font-bold text-lg">{policy.metadata?.name}</h3>
                        <span className="px-2 py-1 text-xs font-mono text-white bg-black">
                          {actions[0] ? actions[0].toUpperCase() : 'UNKNOWN'}
                        </span>
                      </div>
                      <div className="text-xs font-mono text-gray-600 mt-2 space-y-1">
                        <p><span className="font-bold">NS:</span> {policy.spec?.namespaces?.join(', ')}</p>
                        <p><span className="font-bold text-red-600">BLOCK:</span> {disallowed.join(', ')}</p>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>

        {/* Right Column: Visualization & Live Feed */}
        <div className="lg:col-span-2 flex flex-col gap-6">

          {/* The Map */}
          <div className="border-2 border-black p-6">
            <h2 className="text-2xl font-bold mb-2">Global Network Topography</h2>
            <p className="text-gray-500 text-sm mb-4">Packet destinations evaluated by eBPF.</p>

            <div className="bg-gray-50 border border-gray-200 overflow-hidden flex items-center justify-center h-[350px]">
              <ComposableMap projectionConfig={{ scale: 140 }} width={800} height={400} style={{ width: "100%", height: "100%" }}>
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

          {/* Live Violations Feed */}
          <div className="border-2 border-black p-6 bg-black text-white h-[300px] overflow-y-auto">
            <h2 className="text-xl font-bold mb-4 font-mono">Terminal Output // Violations</h2>
            {events.length === 0 ? (
              <p className="text-gray-500 font-mono text-sm animate-pulse">Awaiting network traffic...</p>
            ) : (
              <ul className="space-y-3 font-mono text-sm">
                {events.map((ev, idx) => (
                  <li key={ev.metadata?.uid || idx} className="border-l-2 border-red-500 pl-3 py-1">
                    <div className="flex justify-between text-gray-400 text-xs mb-1">
                      <span>{new Date(ev.lastTimestamp || ev.eventTime).toLocaleTimeString()}</span>
                      <span>POD: {ev.involvedObject?.name}</span>
                    </div>
                    <span className="text-red-400 font-bold">BLOCKED: </span>
                    {ev.message}
                  </li>
                ))}
              </ul>
            )}
          </div>

        </div>
      </div>
    </div>
  );
}

export default App;