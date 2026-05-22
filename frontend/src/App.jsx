import { useState, useEffect } from 'react';
import axios from 'axios';

function App() {
  const [policies, setPolicies] = useState([]);
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(true);

  // Policy Form State (Handles both Create and Edit modes)
  const [formData, setFormData] = useState({
    name: '',
    namespace: 'default',
    country: '',
    action: 'block-kill'
  });
  const [isEditing, setIsEditing] = useState(false);

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

    // Poll for new violations every 3 seconds
    const interval = setInterval(fetchViolations, 3000);
    return () => clearInterval(interval);
  }, []);

  // CRUD: Create or Update Policy
  const handleSubmitPolicy = async (e) => {
    e.preventDefault();
    try {
      if (isEditing) {
        // Submit modifications to PUT endpoint
        await axios.put(`/api/policies/${formData.name}`, {
          namespace: formData.namespace,
          country: formData.country,
          action: formData.action
        });
      } else {
        // Create a new policy via POST
        await axios.post('/api/policies', formData);
      }

      // Reset State
      setFormData({ name: '', namespace: 'default', country: '', action: 'block-kill' });
      setIsEditing(false);
      fetchPolicies();
    } catch (error) {
      console.error("Failed to submit policy:", error);
      alert("Error saving policy. Check your backend console parameters.");
    }
  };

  // CRUD: Delete
  const handleDeletePolicy = async (policyName) => {
    if (!window.confirm(`Are you sure you want to delete policy "${policyName}"?`)) return;
    try {
      await axios.delete(`/api/policies/${policyName}`);
      fetchPolicies();
    } catch (error) {
      console.error("Failed to delete policy:", error);
    }
  };

  // Populate form with current values to trigger edit mode
  const startEditPolicy = (policy) => {
    setIsEditing(true);
    setFormData({
      name: policy.metadata?.name || '',
      namespace: policy.spec?.namespaces?.[0] || 'default',
      country: policy.spec?.disallowedCountries?.[0] || '',
      action: policy.spec?.actions?.[0] || 'block-kill'
    });
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const cancelEdit = () => {
    setIsEditing(false);
    setFormData({ name: '', namespace: 'default', country: '', action: 'block-kill' });
  };

  // Helper: Filter logs that match a specific policy name
  const getViolationsForPolicy = (policyName) => {
    return events.filter(ev => {
      // Check if the policy is mentioned explicitly in the message string
      const messageLower = (ev.message || '').toLowerCase();
      return messageLower.includes(policyName.toLowerCase());
    });
  };

  return (
    <div style={{ maxW: '800px', margin: '0 auto', padding: '20px', fontFamily: 'monospace' }}>

      {/* Header */}
      <header style={{ borderBottom: '2px solid black', paddingBottom: '10px', marginBottom: '20px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h1 style={{ margin: 0, fontSize: '24px' }}>Sovereign Sensor Dashboard</h1>
        </div>
        
      </header>

      {/* Policy Management Form */}
      <section style={{ border: '1px solid black', padding: '15px', marginBottom: '20px', backgroundColor: '#f9f9f9' }}>
        <h2 style={{ margin: '0 0 10px 0', fontSize: '16px' }}>
          {isEditing ? `✏️ MODIFY POLICY: ${formData.name}` : '➕ CREATE NEW SOVEREIGNTY POLICY'}
        </h2>

        <form onSubmit={handleSubmitPolicy}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: '10px', marginBottom: '10px' }}>
            <div>
              <label style={{ display: 'block', fontSize: '12px', fontWeight: 'bold' }}>Policy Name</label>
              <input
                required
                type="text"
                disabled={isEditing} // Name cannot change on PUT
                placeholder="e.g., block-ru"
                style={{ width: '100%', padding: '5px', boxSizing: 'border-box', border: '1px solid #999' }}
                value={formData.name}
                onChange={e => setFormData({ ...formData, name: e.target.value })}
              />
            </div>
            <div>
              <label style={{ display: 'block', fontSize: '12px', fontWeight: 'bold' }}>Target Namespace</label>
              <input
                required
                type="text"
                style={{ width: '100%', padding: '5px', boxSizing: 'border-box', border: '1px solid #999' }}
                value={formData.namespace}
                onChange={e => setFormData({ ...formData, namespace: e.target.value })}
              />
            </div>
            <div>
              <label style={{ display: 'block', fontSize: '12px', fontWeight: 'bold' }}>Country Code (ISO-2)</label>
              <input
                required
                type="text"
                maxLength="2"
                placeholder="e.g., RU"
                style={{ width: '100%', padding: '5px', boxSizing: 'border-box', border: '1px solid #999' }}
                value={formData.country}
                onChange={e => setFormData({ ...formData, country: e.target.value.toUpperCase() })}
              />
            </div>
            <div>
              <label style={{ display: 'block', fontSize: '12px', fontWeight: 'bold' }}>Action Strategy</label>
              <select
                style={{ width: '100%', padding: '5px', boxSizing: 'border-box', border: '1px solid #999' }}
                value={formData.action}
                onChange={e => setFormData({ ...formData, action: e.target.value })}
              >
                <option value="block-kill">block-kill</option>
                <option value="block-noconn">block-noconn</option>
                <option value="log">log</option>
              </select>
            </div>
          </div>

          <div style={{ display: 'flex', gap: '10px' }}>
            <button type="submit" style={{ padding: '6px 12px', cursor: 'pointer', background: '#333', color: 'white', border: 'none', fontWeight: 'bold' }}>
              {isEditing ? 'SAVE CHANGES' : 'APPLY POLICY'}
            </button>
            {isEditing && (
              <button type="button" onClick={cancelEdit} style={{ padding: '6px 12px', cursor: 'pointer', background: '#ccc', color: 'black', border: 'none' }}>
                Cancel
              </button>
            )}
          </div>
        </form>
      </section>

      {/* Accordion List */}
      <main>
        <h2 style={{ fontSize: '18px', borderBottom: '1px solid black', paddingBottom: '5px' }}>Active System Policies</h2>

        {loading ? (
          <p>Gathering cluster object definitions...</p>
        ) : policies.length === 0 ? (
          <p style={{ color: '#666', fontStyle: 'italic' }}>No sovereignty rules are loaded into the API server mesh.</p>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
            {policies.map((policy) => {
              const pName = policy.metadata?.name;
              const policyLogs = getViolationsForPolicy(pName);

              return (
                <details
                  key={policy.metadata?.uid || pName}
                  style={{ border: '1px solid black', padding: '10px', backgroundColor: '#fff' }}
                >
                  <summary style={{ cursor: 'pointer', fontWeight: 'bold', fontSize: '14px', userSelect: 'none' }}>
                    📦 {pName} <span style={{ color: '#666', fontWeight: 'normal', fontSize: '12px' }}>
                      ({policy.spec?.namespaces?.join(', ') || 'all-ns'} ➡️ {policy.spec?.disallowedCountries?.join(', ')})
                    </span>
                  </summary>

                  <div style={{ marginTop: '12px', borderTop: '1px dashed #ccc', paddingTop: '10px' }}>

                    {/* Actions and Metadata block */}
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: '10px', marginBottom: '15px' }}>
                      <div style={{ fontSize: '12px', lineHeight: '1.5' }}>
                        <div><strong>Target Namespaces:</strong> {policy.spec?.namespaces?.join(', ')}</div>
                        <div><strong>Blocked Countries:</strong> {policy.spec?.disallowedCountries?.join(', ')}</div>
                        <div><strong>Enforcement Action:</strong> <span style={{ textTransform: 'uppercase', background: '#eee', padding: '2px 4px' }}>{policy.spec?.actions?.[0] || 'LOG'}</span></div>
                        <div style={{ color: '#777', fontSize: '11px', marginTop: '4px' }}>UID: {policy.metadata?.uid}</div>
                      </div>

                      <div style={{ display: 'flex', gap: '8px' }}>
                        <button
                          onClick={() => startEditPolicy(policy)}
                          style={{ padding: '4px 8px', background: 'none', border: '1px solid black', cursor: 'pointer', fontSize: '12px' }}
                        >
                          Modify
                        </button>
                        <button
                          onClick={() => handleDeletePolicy(pName)}
                          style={{ padding: '4px 8px', background: '#ff4444', color: 'white', border: 'none', cursor: 'pointer', fontSize: '12px', fontWeight: 'bold' }}
                        >
                          Delete
                        </button>
                      </div>
                    </div>

                    {/* Isolated Logs Context Feed */}
                    <div style={{ background: '#111', color: '#00ff00', padding: '10px', fontSize: '12px', overflowY: 'auto', maxH: '180px' }}>
                      <div style={{ color: '#aaa', borderBottom: '1px solid #333', paddingBottom: '4px', marginBottom: '6px', fontWeight: 'bold' }}>
                        📋 Recent Violation Telemetry ({policyLogs.length})
                      </div>

                      {policyLogs.length === 0 ? (
                        <div style={{ color: '#666', fontStyle: 'italic' }}>No dropped transactions or violations registered for this selector.</div>
                      ) : (
                        <ul style={{ listStyleType: 'none', padding: 0, margin: 0 }}>
                          {policyLogs.map((ev, index) => (
                            <li key={ev.metadata?.uid || index} style={{ marginBottom: '6px', borderBottom: '1px solid #222', paddingBottom: '4px' }}>
                              <span style={{ color: '#888' }}>[{new Date(ev.lastTimestamp || ev.eventTime).toLocaleTimeString()}]</span>{' '}
                              <span style={{ color: '#ff5555' }}>POD: {ev.involvedObject?.name || 'unknown'}</span> — {ev.message}
                            </li>
                          ))}
                        </ul>
                      )}
                    </div>

                  </div>
                </details>
              );
            })}
          </div>
        )}
      </main>
    </div>
  );
}

export default App;