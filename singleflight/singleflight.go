package singleflight

import "sync"

// call 正在进行或已结束的请求
type call struct {
	wg sync.WaitGroup
	val any
	err error
}

// Group 管理所有的请求
type Group struct {
	m sync.Map // 并发安全的映射
}

// Do 针对相同的key，保证多次调用Do()，都只会调用一次f()
func (g *Group) Do(key string, f func() (any, error)) (any, error) {
	//检查是否已经存在该键对应的请求
	if existing, ok := g.m.Load(key); ok {
		c := existing.(*call) // 转换为 *call 类型
		c.wg.Wait()
		return c.val, c.err
	}

	// 不存在正在进行的请求，创建新的请求
	c := &call{}
	c.wg.Add(1)
	g.m.Store(key, c)

	// 调用函数
	c.val, c.err = f()
	c.wg.Done()

	g.m.Delete(key)

	return c.val, c.err
}
