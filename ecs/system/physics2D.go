package system

import (
	"math"

	"galaxyzeta.io/engine/base"
	"galaxyzeta.io/engine/collision"
	"galaxyzeta.io/engine/core"
	"galaxyzeta.io/engine/ecs/component"
	"galaxyzeta.io/engine/graphics"
	cc "galaxyzeta.io/engine/infra/concurrency"
	"galaxyzeta.io/engine/infra/logger"
	"galaxyzeta.io/engine/linalg"
)

var NamePhysics2DSystem = "sys_Physics2D"

// PhysicalComponentWrapper wraps RigidBody2D and Transform component.
type PhysicalComponentWrapper struct {
	*component.RigidBody2D
	*component.Transform2D
	*component.PolygonCollider
}

type Physics2DSystem struct {
	*base.SystemBase
	csys     collision.ICollisionSystem
	obj2data map[base.IGameObject2D]PhysicalComponentWrapper
	logger   *logger.Logger
}

func NewPhysics2DSystem(prioriy int, csys collision.ICollisionSystem) *Physics2DSystem {
	return &Physics2DSystem{
		obj2data:   make(map[base.IGameObject2D]PhysicalComponentWrapper, 64),
		SystemBase: base.NewSystemBase(prioriy),
		csys:       csys,
		logger:     logger.New("Physics2D"),
	}
}

func (s *Physics2DSystem) execute(item PhysicalComponentWrapper) {
	linkedList := item.RigidBody2D.GetSpeedList()
	var dx, dy float64
	for element := linkedList.Front(); element != nil; element = element.Next() {
		val := element.Value.(component.SpeedVector)
		deg := linalg.Deg2Rad(linalg.InvertDeg(val.Direction))
		dx += val.Speed * math.Cos(deg)
		dy += val.Speed * math.Sin(deg)
		core.RenderCmdChan <- func() {
			graphics.DrawSegment(linalg.NewSegmentf64(item.X(), item.Y(), item.X()+dx*10, item.Y()+dy*10), linalg.NewRgbaF64(0, 1, 0, 1))
		}
		// do speed atten
		if val.Speed > 0 {
			val.Speed -= val.Acceleration
			if val.Speed < 0 {
				s.logger.Debugf("remove force vector = %v", element)
				linkedList.Remove(element)
				continue
			}
		}
		element.Value = val

	}
	// constant gravity effect
	if item.UseGravity {
		// judge should apply gravity
		gdeg := linalg.Deg2Rad(linalg.InvertDeg(item.GravityVector.Direction))
		gdx := item.GravityVector.Speed * math.Cos(gdeg)
		gdy := item.GravityVector.Speed * math.Sin(gdeg)
		if collision.HasColliderAtPolygonWithTag(s.csys, item.Collider.Shift(dx+gdx, dy+gdy), "solid") {
			// grounded
			item.GravityVector.Speed = 0
		} else {
			// use gravity
			item.GravityVector.Speed += item.GravityVector.Acceleration
			dx += gdx
			dy += gdy
		}
	}

	// set calculated property
	item.SetHspeed(dx)
	item.SetVspeed(dy)

	// calc position
	if item.PolygonCollider == nil {
		return
	}
	// reject collision caused movement
	if !collision.HasColliderAtPolygonWithTag(s.csys, item.Collider.Shift(dx, 0), "solid") {
		item.Transform2D.Pos.X += dx
	}
	if !collision.HasColliderAtPolygonWithTag(s.csys, item.Collider.Shift(0, dy), "solid") {
		item.Transform2D.Pos.Y += dy
	}
}

// ===== IMPLEMENTATION =====

func (s *Physics2DSystem) Execute(executor *cc.Executor) {
	for _, item := range s.obj2data {
		executor.AsyncExecute(func() (interface{}, error) {
			s.execute(item)
			return nil, nil
		})
	}
}

func (s *Physics2DSystem) GetSystemBase() *base.SystemBase {
	return s.SystemBase
}

func (s *Physics2DSystem) GetName() string {
	return NamePhysics2DSystem
}

func (s *Physics2DSystem) Register(iobj base.IGameObject2D) {
	rb := iobj.GetGameObject2D().GetComponent(component.NameRigidBody2D).(*component.RigidBody2D)
	tf := iobj.GetGameObject2D().GetComponent(component.NameTransform2D).(*component.Transform2D)
	pc := iobj.GetGameObject2D().GetComponent(component.NamePolygonCollider).(*component.PolygonCollider)
	s.obj2data[iobj] = PhysicalComponentWrapper{
		RigidBody2D:     rb,
		Transform2D:     tf,
		PolygonCollider: pc,
	}
}

func (s *Physics2DSystem) Unregister(iobj base.IGameObject2D) {
	delete(s.obj2data, iobj)
}
