import { useState, useEffect, useRef } from 'react';

/**
 * Main application component that handles WebSocket connections and dynamic UI rendering.
 * Supports real-time updates of chat components and theme configurations.
 * @component
 * @returns {JSX.Element} The rendered application
 */
function App() {
  const [uiConfig, setUiConfig] = useState({
    layout: 'light',
    components: [], // Initialize with empty array
    theme: {
      primaryColor: '#ffffff',
      secondaryColor: '#000000',
      fontSize: '16px'
    }
  });
  const [error, setError] = useState(null);
  const [ws, setWs] = useState(null);
  const [pageId, setPageId] = useState(null);
  const messageListRef = useRef(null);

  /**
   * Effect hook to extract and set the pageId from URL parameters.
   * If no pageId is provided, defaults to 'default'.
   */
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const id = params.get('pageId') || 'default';
    console.log('PageID from URL:', id);
    setPageId(id);
  }, []);

  /**
   * Effect hook to establish and manage WebSocket connection.
   * Handles connection lifecycle and message processing.
   * @param {string} pageId - The current page identifier
   */
  useEffect(() => {
    if (!pageId) return;

    console.log('Starting WebSocket connection...');
    
    const DROPLET_URL = '137.184.9.181';
    const wsUrl = `ws://${DROPLET_URL}/ws?pageId=${pageId}`;
    
    console.log('Attempting to connect to:', wsUrl);
    const websocket = new WebSocket(wsUrl);

    websocket.onopen = () => {
      console.log('WebSocket Connected!');
    };

    websocket.onmessage = (event) => {
      try {
        const config = JSON.parse(event.data);
        console.log('Received config:', config);
        setUiConfig(config);
        setError(null);
      } catch (err) {
        console.error('WebSocket message error:', err);
        setError('Failed to process server update');
      }
    };

    websocket.onerror = (error) => {
      console.error('WebSocket error:', error);
      setError('Failed to connect to server');
      console.log('Connection failed to:', wsUrl);
    };

    websocket.onclose = (event) => {
      console.log('WebSocket disconnected:', event.code, event.reason);
    };

    setWs(websocket);

    return () => {
      console.log('Cleaning up WebSocket...');
      websocket.close();
    };
  }, [pageId]);

  /**
   * Renders the chat header component with user information.
   * @param {Object} component - The component configuration object
   * @param {Object} component.properties - Component properties containing user details
   * @param {string} component.properties.userName - The display name of the user
   * @param {string} component.properties.userStatus - The current status of the user
   * @returns {JSX.Element|null} Rendered chat header or null if invalid props
   */
  const renderChatHeader = (component) => {
    if (!component?.properties) return null;
    
    return (
      <div key={component.id} className="flex items-center p-4 border-b">
        <div className="flex-1">
          <h3 className="font-bold">{component.properties.userName || 'Unknown User'}</h3>
          <span className="text-sm text-gray-500">{component.properties.userStatus || 'Status unknown'}</span>
        </div>
      </div>
    );
  };

  /**
   * Renders the chat messages component with message history.
   * @param {Object} component - The component configuration object
   * @param {Object} component.properties - Component properties
   * @param {string} component.properties.messages - JSON string of messages
   * @returns {JSX.Element|null} Rendered message list or null if invalid props
   */
  const renderChatMessages = (component) => {
    if (!component?.properties?.messages) return null;
    
    let messages = [];
    try {
      messages = JSON.parse(component.properties.messages);
    } catch (e) {
      console.error('Failed to parse messages:', e);
    }
    
    return (
      <div key={component.id} className="flex-1 p-4 overflow-y-auto" ref={messageListRef}>
        {Array.isArray(messages) ? messages.map((message) => (
          <div 
            key={message.id}
            className={`mb-4 ${message.sender === 'user' ? 'text-right' : 'text-left'}`}
          >
            <div 
              className={`inline-block p-3 rounded-lg ${
                message.sender === 'user' 
                  ? 'bg-blue-500 text-white' 
                  : 'bg-gray-200 text-gray-800'
              }`}
            >
              <p>{message.content}</p>
              <span className="text-xs opacity-75">
                {new Date(message.timestamp).toLocaleTimeString()}
              </span>
            </div>
          </div>
        )) : null}
      </div>
    );
  };

  /**
   * Factory function to render different component types based on their configuration.
   * @param {Object} component - The component configuration object
   * @param {string} component.type - The type of component to render
   * @returns {JSX.Element|null} Rendered component or null if type is unsupported
   */
  const renderComponent = (component) => {
    if (!component?.type) return null;

    switch (component.type) {
      case 'chat-header':
        return renderChatHeader(component);
      case 'chat-messages':
        return renderChatMessages(component);
      default:
        return null;
    }
  };

  // Add debug render to see what's happening
  if (error) {
    return (
      <div className="text-red-500 p-4">
        Error: {error}
      </div>
    );
  }

  if (!uiConfig) {
    return (
      <div className="text-gray-500 p-4">
        Loading... (PageID: {pageId})
      </div>
    );
  }

  console.log('Rendering with config:', uiConfig);

  // Add TypeScript-style interface documentation for the expected data structures
  /**
   * @typedef {Object} Message
   * @property {string} id - Unique identifier for the message
   * @property {string} content - The message content
   * @property {string} sender - The sender of the message ('user' or other)
   * @property {string} timestamp - ISO timestamp of when the message was sent
   */

  /**
   * @typedef {Object} UIConfig
   * @property {string} layout - The layout theme ('light' or 'dark')
   * @property {Array<Object>} components - Array of component configurations
   * @property {Object} theme - Theme configuration
   * @property {string} theme.primaryColor - Primary color in hex
   * @property {string} theme.secondaryColor - Secondary color in hex
   * @property {string} theme.fontSize - Base font size with units
   */

  return (
    <div
      className="min-h-screen flex flex-col w-full"
      style={{
        backgroundColor: uiConfig?.layout === 'dark' ? '#1a1a1a' : '#ffffff',
        color: uiConfig?.layout === 'dark' ? '#ffffff' : '#000000',
      }}
    >
      <div className="w-full flex flex-col h-screen">
        {Array.isArray(uiConfig?.components) ? (
          uiConfig.components.map(component => renderComponent(component))
        ) : (
          <div className="text-gray-500 p-4">
            Waiting for components...
          </div>
        )}
      </div>
    </div>
  );
}

export default App;