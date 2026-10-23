// Only the test launcher and fixture clients use these throwaway credentials.
// Browser contexts authenticate normally; never attach the bearer globally.
export const appOrigin = 'http://127.0.0.1:18080';
export const renderOrigin = 'http://127.0.0.1:18081';
export const serviceToken = 'exhibit-e2e-only-token';
export const owner = { username: 'admin', password: 'changeme' };
