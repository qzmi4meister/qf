import axios from 'axios'

const client = axios.create({
  withCredentials: true,
})

client.interceptors.response.use(
  (r) => r,
  (err) => {
    if (err.response?.status === 401) {
      const loginUrl = '/app/login'
      if (window.location.pathname !== loginUrl) {
        window.location.href = loginUrl
      }
    }
    return Promise.reject(err)
  },
)

export default client
